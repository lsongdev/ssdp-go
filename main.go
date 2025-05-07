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
