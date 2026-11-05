package ssdp

import (
	"context"
	"errors"
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
		// Only the NOTIFY should reach the callback.
		sender.Write([]byte("HTTP/1.1 200 OK\r\nid: response\r\n\r\n"))
		sender.Write([]byte("M-SEARCH * HTTP/1.1\r\nid: search\r\n\r\n"))
		_, err = sender.Write([]byte("NOTIFY * HTTP/1.1\r\nNTS: ssdp:alive\r\nid: bulb\r\n\r\n"))
		done <- err
	}()
	client := NewClient(Config{Port: port, Address: "127.0.0.1"})
	var got Packet
	err = client.ListenNotifications(context.Background(), func(packet Packet) bool { got = packet; return true })
	if err != nil || got.Method != MethodNotify || got.Target != "*" || got.Header.Get("id") != "bulb" || got.Source == nil {
		t.Fatalf("packet=%+v err=%v", got, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestListenNotificationsReturnsContextError(t *testing.T) {
	client := NewClient(Config{Port: freePort(t), Address: "127.0.0.1"})
	visit := func(Packet) bool { return false }

	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		err := client.ListenNotifications(ctx, visit)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline: err = %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	if err := client.ListenNotifications(ctx, visit); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: err = %v", err)
	}
	if err := client.ListenNotifications(context.Background(), nil); err == nil {
		t.Fatal("accepted nil callback")
	}
}
