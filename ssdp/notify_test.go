package ssdp

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestListenNotifications(t *testing.T) {
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	done := make(chan error, 1)
	go func() {
		sender, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			done <- err
			return
		}
		defer sender.Close()
		time.Sleep(20 * time.Millisecond)
		_, err = sender.Write([]byte("NOTIFY * HTTP/1.1\r\nNTS: ssdp:alive\r\nid: bulb\r\n\r\n"))
		done <- err
	}()
	client := NewClient(&Config{Port: port, Broadcast: "127.0.0.1"})
	var got Packet
	err = client.ListenNotifications(context.Background(), time.Second, func(packet Packet) bool { got = packet; return true })
	if err != nil || got.StartLine != "NOTIFY * HTTP/1.1" || got.Header.Get("id") != "bulb" || got.Source == nil {
		t.Fatalf("packet=%+v err=%v", got, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
