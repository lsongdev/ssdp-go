// Package ssdp is an IPv4 SSDP control point: it searches for devices with
// M-SEARCH and listens for NOTIFY advertisements. It follows the discovery part
// of UPnP Device Architecture 2.0 and also works with SSDP-like vendor protocols
// that use other ports, addresses and headers. It does not advertise or answer
// searches as a device, and it does not support IPv6.
package ssdp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/net/ipv4"
)

const (
	MethodSearch = "M-SEARCH"
	MethodNotify = "NOTIFY"
)

// Defaults applied by NewClient to zero Config fields.
const (
	DefaultAddress = "239.255.255.250"
	DefaultPort    = 1900
	DefaultMX      = 3
	DefaultTTL     = 2
	DefaultSends   = 3
)

const (
	sendInterval = 200 * time.Millisecond
	maxDatagram  = 65507
)

// Config configures a Client. Zero fields select the defaults.
type Config struct {
	// Port is the destination port. The default is DefaultPort.
	Port int
	// Address is the destination IPv4 multicast, broadcast or unicast address.
	// The default is DefaultAddress.
	Address string
	// MX is the maximum response delay requested from devices, in seconds.
	// The default is DefaultMX.
	MX int
	// TTL is the IP time-to-live of multicast searches. The default is DefaultTTL.
	TTL int
	// Sends is how many times each search is sent, because UDP is unreliable.
	// The default is DefaultSends.
	Sends int
	// Window is how long a search collects responses. The default is MX plus
	// two seconds. A context can end a search earlier but never extends it.
	Window time.Duration
	// Interfaces selects the interfaces used for multicast. The default is every
	// interface that is up, is not loopback, supports multicast and has an IPv4
	// address.
	Interfaces []net.Interface
	// Headers adds search request headers, such as USER-AGENT or CPFN.UPNP.ORG.
	Headers map[string]string
}

// Client is safe for concurrent use.
type Client struct {
	config Config
}

// NewClient creates a Client.
func NewClient(config Config) *Client {
	if config.Port == 0 {
		config.Port = DefaultPort
	}
	if config.Address == "" {
		config.Address = DefaultAddress
	}
	if config.MX <= 0 {
		config.MX = DefaultMX
	}
	if config.TTL <= 0 {
		config.TTL = DefaultTTL
	}
	if config.Sends <= 0 {
		config.Sends = DefaultSends
	}
	if config.Window <= 0 {
		config.Window = time.Duration(config.MX+2) * time.Second
	}
	return &Client{config: config}
}

// Search sends M-SEARCH and returns the responses received during the search
// window. See SearchFunc for the rules that apply.
func (c *Client) Search(ctx context.Context, searchType string) ([]Packet, error) {
	var out []Packet
	err := c.SearchFunc(ctx, searchType, func(packet Packet) bool {
		out = append(out, packet)
		return false
	})
	return out, err
}

// SearchFunc sends M-SEARCH and calls visit as each new successful response
// arrives, until visit returns true or the search ends. An empty searchType
// means ssdp:all. Responses that repeat a source, USN, ST and Location are
// dropped. ST is not checked, since vendor protocols may omit it.
//
// The search ends without error when Config.Window elapses or a context deadline
// passes. It returns the context's error if ctx is canceled, or if ctx has
// already ended when SearchFunc is called.
func (c *Client) SearchFunc(ctx context.Context, searchType string, visit func(Packet) bool) error {
	if visit == nil {
		return errors.New("ssdp: search callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if searchType == "" {
		searchType = "ssdp:all"
	}
	destination, err := c.destination()
	if err != nil {
		return err
	}
	req := c.searchRequest(searchType)
	if err := req.Validate(); err != nil {
		return err
	}
	payload := req.Bytes()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return fmt.Errorf("ssdp: listen: %w", err)
	}
	defer conn.Close()
	send, err := c.newSender(conn, destination)
	if err != nil {
		return err
	}
	if err := send(payload); err != nil {
		return fmt.Errorf("ssdp: send search: %w", err)
	}

	searchCtx, cancel := context.WithTimeout(ctx, c.config.Window)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(sendInterval)
		defer ticker.Stop()
		for i := 1; i < c.config.Sends; i++ {
			select {
			case <-searchCtx.Done():
				return
			case <-ticker.C:
				// The first send proved the socket works, and a later
				// failure only loses a redundant copy.
				_ = send(payload)
			}
		}
	}()
	defer func() {
		cancel()
		<-done
	}()

	seen := make(map[string]bool)
	err = readPackets(searchCtx, conn, func(packet Packet) bool {
		if packet.StatusCode != 200 {
			return false
		}
		key := responseKey(packet)
		if seen[key] {
			return false
		}
		seen[key] = true
		return visit(packet)
	})
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	if searchCtx.Err() != nil {
		return nil
	}
	return fmt.Errorf("ssdp: read: %w", err)
}

func (c *Client) searchRequest(searchType string) *Request {
	req := NewRequest(MethodSearch, "*")
	for name, value := range c.config.Headers {
		req.AddHeader(name, value)
	}
	req.Host = net.JoinHostPort(c.config.Address, strconv.Itoa(c.config.Port))
	req.AddHeader("MAN", `"ssdp:discover"`)
	req.AddHeader("MX", strconv.Itoa(c.config.MX))
	req.AddHeader("ST", searchType)
	return req
}

func (c *Client) destination() (*net.UDPAddr, error) {
	ip := net.ParseIP(c.config.Address)
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("ssdp: invalid IPv4 destination address %q", c.config.Address)
	}
	return &net.UDPAddr{IP: ip, Port: c.config.Port}, nil
}

// newSender returns a function that sends a datagram to destination. For
// multicast it sends a copy on every interface, since the default route alone
// can pick a VPN or container interface and miss the devices. It fails only if
// no copy could be sent, as some interfaces cannot reach the group.
func (c *Client) newSender(conn *net.UDPConn, destination *net.UDPAddr) (func([]byte) error, error) {
	if !destination.IP.IsMulticast() {
		return func(payload []byte) error {
			_, err := conn.WriteToUDP(payload, destination)
			return err
		}, nil
	}
	packetConn := ipv4.NewPacketConn(conn)
	if err := packetConn.SetMulticastTTL(c.config.TTL); err != nil {
		return nil, fmt.Errorf("ssdp: set multicast TTL: %w", err)
	}
	interfaces := c.multicastInterfaces()
	return func(payload []byte) error {
		if len(interfaces) == 0 {
			_, err := conn.WriteToUDP(payload, destination)
			return err
		}
		var errs []error
		sent := false
		for i := range interfaces {
			err := packetConn.SetMulticastInterface(&interfaces[i])
			if err == nil {
				_, err = conn.WriteToUDP(payload, destination)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", interfaces[i].Name, err))
				continue
			}
			sent = true
		}
		if sent {
			return nil
		}
		return errors.Join(errs...)
	}, nil
}

// multicastInterfaces returns the configured interfaces or the usable ones. An
// empty result means the system default is used.
func (c *Client) multicastInterfaces() []net.Interface {
	if len(c.config.Interfaces) > 0 {
		return c.config.Interfaces
	}
	all, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var usable []net.Interface
	for _, ifi := range all {
		if ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagMulticast != 0 && ifi.Flags&net.FlagLoopback == 0 && hasIPv4(ifi) {
			usable = append(usable, ifi)
		}
	}
	return usable
}

func hasIPv4(ifi net.Interface) bool {
	addrs, err := ifi.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			return true
		}
	}
	return false
}

func responseKey(packet Packet) string {
	source := ""
	if packet.Source != nil {
		source = packet.Source.IP.String()
	}
	return source + "\x00" + packet.Header.Get("USN") + "\x00" + packet.Header.Get("ST") + "\x00" + packet.Header.Get("Location")
}

// readPackets calls visit for each valid datagram until visit returns true,
// which returns nil, or reading fails, which includes ctx ending. Using only the
// context to interrupt the read means ctx.Err is always set by then.
func readPackets(ctx context.Context, conn *net.UDPConn, visit func(Packet) bool) error {
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Now()) })
	defer stop()
	buf := make([]byte, maxDatagram)
	for {
		n, source, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		packet, err := ParsePacket(buf[:n], source)
		if err != nil {
			continue
		}
		if visit(packet) {
			return nil
		}
	}
}
