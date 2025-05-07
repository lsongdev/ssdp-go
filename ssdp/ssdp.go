// SSDP (Simple Service Discovery Protocol) package provides an implementation of the SSDP
// specification.
package ssdp

import (
	"fmt"
	"net"
	"time"
)

const (
	MethodSearch = "M-SEARCH"
	MethodNoify  = "NOTIFY"
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

// Search the network for SSDP devices using the given search string and duration
// to discover new devices. This function will return an array of SearchReponses
// discovered.
// https://datatracker.ietf.org/doc/html/draft-cai-ssdp-v1-03
func (c *Client) Search(searchType string) (out []*SearchResponse, err error) {
	if searchType == "" {
		searchType = "ssdp:all"
	}
	conn, err := c.Listen()
	if err != nil {
		return
	}
	defer conn.Close()
	req := NewRequest(MethodSearch, "*")
	req.Host = fmt.Sprintf("%s:%d", c.config.Broadcast, c.config.Port)
	req.AddHeader("ST", searchType)
	req.AddHeader("MX", "3")
	_, err = conn.WriteTo(req.Bytes(), req.Address())
	if err != nil {
		return
	}
	// Only listen for responses for duration amount of time.
	duration := time.Duration(3) * time.Second
	conn.SetReadDeadline(time.Now().Add(duration))
	responses, err := readResponses(conn)
	if err != nil {
		return
	}
	for _, response := range responses {
		out = append(out, &SearchResponse{
			Type:     response.Headers["ST"],
			USN:      response.Headers["USN"],
			Ext:      response.Headers["EXT"],
			Server:   response.Headers["SERVER"],
			Location: response.Headers["LOCATION"],
			Headers:  response.Headers,
		})
	}
	return
}
