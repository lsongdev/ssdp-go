package main

import (
	"fmt"

	"github.com/song940/ssdp-go/ssdp"
)

func main() {
	client := ssdp.NewClient(&ssdp.Config{
		Port: 1982,
	})
	responses, err := client.Search("wifi_bulb")
	if err != nil {
		panic(err)
	}
	for _, r := range responses {
		fmt.Println(r.Location)
	}
}
