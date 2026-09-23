// SSDP (Simple Service Discovery Protocol) package provides an implementation of the SSDP
// specification.
package ssdp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	MethodSearch = "M-SEARCH"
	MethodNoify  = "NOTIFY" // deprecated: use MethodNotify
	MethodNotify = "NOTIFY"
)

type Config struct {
	Port      int
	Broadcast string
}

type Client struct {
	config *Config
}

// Create a new Client
func NewClient(config *Config) *Client {
	if config == nil {
		config = &Config{}
	}
	if config.Port == 0 {
		config.Port = 1900
	}
	if config.Broadcast == "" {
		config.Broadcast = "239.255.255.250"
	}
	return &Client{config: config}
}

// The search response from a device implementing SSDP.
type SearchResponse struct {
	Ext      string
	USN      string
	Type     string
	Location string
	Server   string
	Headers  map[string]string
}

func (c *Client) Listen() (*net.UDPConn, error) {
	address := fmt.Sprintf("0.0.0.0:%d", c.config.Port)
	serverAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	return net.ListenUDP("udp", serverAddr)
}

// Search keeps the original API and searches for five seconds.
func (c *Client) Search(searchType string) ([]*SearchResponse, error) {
	return c.SearchContext(context.Background(), searchType, 5*time.Second)
}

// SearchContext sends M-SEARCH and collects unicast responses for window.
// The sending socket uses an ephemeral source port, as required for concurrent
// discovery clients and for listening alongside a passive notification receiver.
func (c *Client) SearchContext(ctx context.Context, searchType string, window time.Duration) (out []*SearchResponse, err error) {
	if window <= 0 {
		return nil, fmt.Errorf("ssdp: search window must be positive")
	}
	if searchType == "" {
		searchType = "ssdp:all"
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	req := NewRequest(MethodSearch, "*")
	req.Host = fmt.Sprintf("%s:%d", c.config.Broadcast, c.config.Port)
	req.AddHeader("ST", searchType)
	req.AddHeader("MX", "3")
	target := &net.UDPAddr{IP: net.ParseIP(c.config.Broadcast), Port: c.config.Port}
	if target.IP == nil {
		return nil, fmt.Errorf("ssdp: invalid broadcast address %q", c.config.Broadcast)
	}
	if _, err := conn.WriteToUDP(req.Bytes(), target); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(window)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Now()) })
	defer stop()
	buf := make([]byte, 65507)
	for {
		n, source, readErr := conn.ReadFromUDP(buf)
		if readErr != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			var netErr net.Error
			if errors.As(readErr, &netErr) && netErr.Timeout() {
				return out, nil
			}
			return out, readErr
		}
		packet, parseErr := ParsePacket(buf[:n], source)
		if parseErr != nil || packet.StartLine != "HTTP/1.1 200 OK" {
			continue
		}
		out = append(out, &SearchResponse{
			Type: packet.Header.Get("ST"), USN: packet.Header.Get("USN"),
			Ext: packet.Header.Get("EXT"), Server: packet.Header.Get("SERVER"),
			Location: packet.Header.Get("LOCATION"), Headers: legacyHeaders(packet.Header),
		})
	}
}

func legacyHeaders(header http.Header) map[string]string {
	result := make(map[string]string, len(header))
	for name := range header {
		result[strings.ToUpper(name)] = header.Get(name)
	}
	return result
}
