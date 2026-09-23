package ssdp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// ListenNotifications listens for NOTIFY datagrams for a bounded window.
// The callback can return true to stop early. The socket closes before return.
// For multicast profiles, Broadcast must be a multicast group; for devices
// using IPv4 broadcast (such as Bambu), set Broadcast to 255.255.255.255.
func (c *Client) ListenNotifications(ctx context.Context, window time.Duration, visit func(Packet) bool) error {
	if window <= 0 {
		return fmt.Errorf("ssdp: listening window must be positive")
	}
	if visit == nil {
		return fmt.Errorf("ssdp: notification callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	addr := &net.UDPAddr{IP: net.ParseIP(c.config.Broadcast), Port: c.config.Port}
	if addr.IP == nil || addr.IP.To4() == nil {
		return fmt.Errorf("ssdp: invalid IPv4 broadcast/group address %q", c.config.Broadcast)
	}
	var conn *net.UDPConn
	var err error
	if addr.IP.IsMulticast() {
		conn, err = net.ListenMulticastUDP("udp4", nil, addr)
	} else {
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{Port: c.config.Port})
	}
	if err != nil {
		return fmt.Errorf("ssdp: listen: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(window)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetReadDeadline(time.Now()) })
	defer stop()
	buf := make([]byte, 65507)
	for {
		n, source, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return nil
			}
			return err
		}
		packet, err := ParsePacket(buf[:n], source)
		if err != nil || packet.StartLine != "NOTIFY * HTTP/1.1" {
			continue
		}
		if visit(packet) {
			return nil
		}
	}
}
