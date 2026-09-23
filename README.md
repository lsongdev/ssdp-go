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
  "fmt"

  "github.com/lsongdev/ssdp-go/ssdp"
)

func main() {
  client := ssdp.NewClient(&ssdp.Config{})
  responses, err := client.Search("")
  if err != nil {
    panic(err)
  }
  for _, r := range responses {
    fmt.Println(r.Type, r.USN, r.Location)
  }
}
```

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

`Search` preserves the original API. `SearchContext` provides a bounded,
cancellable search using an ephemeral source port. `ListenNotifications`
receives passive advertisements for a bounded window and closes its socket
before returning. `ParsePacket` exposes case-insensitive headers and the UDP
source address so device libraries can interpret vendor fields.

```go
client := ssdp.NewClient(&ssdp.Config{Port: 2021, Broadcast: "255.255.255.255"})
err := client.ListenNotifications(ctx, 8*time.Second, func(packet ssdp.Packet) bool {
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
