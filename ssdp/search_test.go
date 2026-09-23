package ssdp

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSearch(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		_ = server.SetReadDeadline(time.Now().Add(time.Second))
		n, remote, e := server.ReadFromUDP(buf)
		if e != nil {
			done <- e
			return
		}
		if !strings.Contains(string(buf[:n]), "ST: wifi_bulb") {
			done <- context.Canceled
			return
		}
		_, e = server.WriteToUDP([]byte("HTTP/1.1 200 OK\r\nLocation: yeelight://127.0.0.1:55443\r\nid: bulb\r\n\r\n"), remote)
		done <- e
	}()
	client := NewClient(Config{Port: server.LocalAddr().(*net.UDPAddr).Port, Address: "127.0.0.1"})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	results, err := client.Search(ctx, "wifi_bulb")
	if err != context.DeadlineExceeded || len(results) != 1 || results[0].Header.Get("Location") != "yeelight://127.0.0.1:55443" {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
