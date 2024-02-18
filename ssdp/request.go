package ssdp

import (
	"fmt"
	"net"
	"strings"
)

type Request struct {
	Method  string
	Path    string
	Host    string
	Headers map[string]string
}

func NewRequest(method, path string) *Request {
	return &Request{
		Method: method,
		Path:   path,
		Headers: map[string]string{
			"Man": `"ssdp:discover"`,
		},
	}
}

func (p *Request) AddHeader(name, value string) {
	p.Headers[name] = value
}

func (p *Request) String() string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%s %s HTTP/1.1", p.Method, p.Path))
	lines = append(lines, fmt.Sprintf("Host: %s", p.Host))
	for name, value := range p.Headers {
		lines = append(lines, name+": "+value)
	}
	lines = append(lines, "")
	// lines = append(lines, "")
	return strings.Join(lines, "\r\n")
}

func (p *Request) Bytes() []byte {
	return []byte(p.String())
}

func (p *Request) Address() *net.UDPAddr {
	address, err := net.ResolveUDPAddr("udp", p.Host)
	if err != nil {
		panic("Could not resolve address: " + err.Error())
	}
	return address
}
