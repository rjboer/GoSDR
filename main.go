package main

import (
	"fmt"
	"log"

	"github.com/rjboer/GoSDR/iiod"
)

func main() {
	c, err := iiod.Dial("192.168.2.1:30431")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Printf("failed to close IIOD client: %v", err)
		}
	}()

	info, err := c.GetContextInfo()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("IIOD VERSION: %d.%d %s\n", info.Major, info.Minor, info.Description)
}
