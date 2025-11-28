package main

import (
	"fmt"
	"log"

	"yourmodule/iiod"
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

	reply, err := c.Send("VERSION")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("IIOD VERSION:", reply)
}
