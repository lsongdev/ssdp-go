package ssdp

import (
	"context"
	"errors"
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
)

// ListenNotifications calls visit for each NOTIFY datagram until visit returns
// true, which returns nil, or ctx ends, which returns ctx.Err(). The socket is
// closed before it returns.
//
// For a multicast Address it joins the group on every interface where that is
// possible (see Config.Interfaces). For any other Address, such as the broadcast
// address 255.255.255.255, it listens on Port on all interfaces.
func (c *Client) ListenNotifications(ctx context.Context, visit func(Packet) bool) error {
	if visit == nil {
		return errors.New("ssdp: notification callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	destination, err := c.destination()
	if err != nil {
		return err
	}
	conn, err := c.listen(destination)
	if err != nil {
		return fmt.Errorf("ssdp: listen: %w", err)
	}
	defer conn.Close()
	err = readPackets(ctx, conn, func(packet Packet) bool {
		return packet.Method == MethodNotify && visit(packet)
	})
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("ssdp: read: %w", err)
}

func (c *Client) listen(destination *net.UDPAddr) (*net.UDPConn, error) {
	if !destination.IP.IsMulticast() {
		return net.ListenUDP("udp4", &net.UDPAddr{Port: destination.Port})
	}
	interfaces := c.multicastInterfaces()
	if len(interfaces) == 0 {
		return net.ListenMulticastUDP("udp4", nil, destination)
	}
	var errs []error
	for i := range interfaces {
		// ListenMulticastUDP allows the port to be shared with other listeners.
		conn, err := net.ListenMulticastUDP("udp4", &interfaces[i], destination)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", interfaces[i].Name, err))
			continue
		}
		packetConn := ipv4.NewPacketConn(conn)
		group := &net.UDPAddr{IP: destination.IP}
		for j := i + 1; j < len(interfaces); j++ {
			// An interface that cannot join simply receives nothing.
			_ = packetConn.JoinGroup(&interfaces[j], group)
		}
		return conn, nil
	}
	return nil, errors.Join(errs...)
}
