// SSDP (Simple Service Discovery Protocol) package provides an implementation of the SSDP
// specification.
package ssdp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	MethodSearch = "M-SEARCH"
	MethodNotify = "NOTIFY"
)

type Config struct {
	Port    int
	Address string
}

type Client struct {
	config Config
}

// Create a new Client
func NewClient(config Config) *Client {
	if config.Port == 0 {
		config.Port = 1900
	}
	if config.Address == "" {
		config.Address = "239.255.255.250"
	}
	return &Client{config: config}
}

// Search sends M-SEARCH and collects responses until the context ends or the
// default five-second search window elapses. A caller may use a shorter deadline.
func (c *Client) Search(ctx context.Context, searchType string) (out []Packet, err error) {
	searchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if searchType == "" {
		searchType = "ssdp:all"
	}
	if err := searchCtx.Err(); err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	req := NewRequest(MethodSearch, "*")
	req.Host = fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	req.AddHeader("ST", searchType)
	req.AddHeader("MX", "3")
	target := &net.UDPAddr{IP: net.ParseIP(c.config.Address), Port: c.config.Port}
	if target.IP == nil {
		return nil, fmt.Errorf("ssdp: invalid broadcast address %q", c.config.Address)
	}
	if _, err := conn.WriteToUDP(req.Bytes(), target); err != nil {
		return nil, err
	}
	if deadline, ok := searchCtx.Deadline(); ok {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
	}
	stop := context.AfterFunc(searchCtx, func() { _ = conn.SetReadDeadline(time.Now()) })
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
		out = append(out, packet)
	}
}
