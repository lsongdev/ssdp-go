# ssdp-go

simple ssdp library in golang

## install

```shell
go get github.com/lsongdev/ssdp-go
```

## example

```go
package main

import (
  "context"
  "fmt"

  "github.com/lsongdev/ssdp-go/ssdp"
)

func main() {
  client := ssdp.NewClient(ssdp.Config{})
  responses, err := client.Search(context.Background(), "")
  if err != nil {
    panic(err)
  }
  for _, r := range responses {
    fmt.Println(r.Header.Get("ST"), r.Header.Get("USN"), r.Header.Get("Location"))
  }
}
```

Use `SearchFunc` to handle responses as they arrive and stop as soon as you have
what you need:

```go
err := client.SearchFunc(ctx, "upnp:rootdevice", func(p ssdp.Packet) bool {
  fmt.Println(p.Header.Get("Location"))
  return true // stop after the first response
})
```

## searching

- Each search is sent `Config.Sends` times (default 3), 200ms apart, on every
  usable interface. `Config.Interfaces` selects specific interfaces.
- Multicast packets use `Config.TTL` (default 2), and requests ask devices to
  reply within `Config.MX` seconds (default 3).
- A search collects responses for `Config.Window` (default MX + 2 seconds). A
  context can end it earlier but cannot extend it.
- The window ending or a context deadline passing is a normal end, not an error.
  If the context is canceled, `Search` returns the responses so far and
  `context.Canceled`. A context that has already ended returns its error at once.
- Non-200 responses and repeats of the same source, USN, ST and Location are
  dropped. ST is not checked, because vendor protocols may omit it.
- `Config.Headers` adds request headers such as `USER-AGENT` or `CPFN.UPNP.ORG`.

## scope

This package is an IPv4 control point. It does not advertise or answer searches
as a device, and it does not support IPv6.

## license

This project is under MIT license.

## Discovery profiles

The package supports the HTTP-over-UDP message format used by SSDP and by
SSDP-like device protocols. The standard UPnP profile uses multicast
`239.255.255.250:1900`, `M-SEARCH` requests, `HTTP/1.1 200 OK` responses,
and `NOTIFY` advertisements. Device protocols can choose different ports and
fields:

| Profile | Destination | Discovery |
| --- | --- | --- |
| UPnP/SSDP | `239.255.255.250:1900` | `M-SEARCH` and `NOTIFY` |
| Yeelight | `239.255.255.250:1982` | `M-SEARCH` and `NOTIFY`; `ST: wifi_bulb` |
| Bambu Lab | IPv4 broadcast UDP `2021` | periodic `NOTIFY`; printer serial in `USN` |

`ListenNotifications` receives passive advertisements until a callback stops it,
which returns nil, or the context ends, which returns the context's error. It then
closes its socket. `ParsePacket` exposes the method or status code, case-insensitive
headers and the UDP source address so device libraries can interpret vendor fields.

```go
client := ssdp.NewClient(ssdp.Config{Port: 2021, Address: "255.255.255.255"})
err := client.ListenNotifications(ctx, func(packet ssdp.Packet) bool {
    if packet.Header.Get("NT") != "urn:bambulab-com:device:3dprinter:1" {
        return false
    }
    fmt.Println(packet.Header.Get("USN"), packet.Source.IP)
    return true // stop after the first matching device
})
```

The library parses the shared envelope; callers must validate device-specific
headers and authenticate subsequent connections. Bambu's broadcast packet is
not itself proof of printer identity.
