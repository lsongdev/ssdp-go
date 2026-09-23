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
