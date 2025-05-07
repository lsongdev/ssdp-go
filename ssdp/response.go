package ssdp

import (
	"fmt"
	"net"
	"strings"
)

type Response struct {
	Protocol   string
	StatusCode string
	StatusText string
	Headers    map[string]string
	RemoteAddr string
}

func NewResponse() *Response {
	return &Response{
		Headers: map[string]string{},
	}
}

func readResponses(conn *net.UDPConn) (out []*Response, err error) {
	buf := make([]byte, 1024)
	for {
		rlen, addr, err := conn.ReadFromUDP(buf)
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			break // duration reached, return what we've found
		}
		response, err := parseResponse(buf[:rlen])
		if err != nil {
			return nil, err
		}
		response.RemoteAddr = addr.String()
		out = append(out, response)
	}
	return
}

func parseResponse(message []byte) (out *Response, err error) {
	lines := strings.Split(string(message), "\r\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("empty response")
	}
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid response: %s", lines[0])
	}
	out = NewResponse()
	out.Protocol = strings.TrimSpace(parts[0])
	out.StatusCode = strings.TrimSpace(parts[1])
	out.StatusText = strings.TrimSpace(parts[2])
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		out.Headers[strings.ToUpper(name)] = value
	}
	return
}
