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

	reply, err := c.Send("VERSION")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("IIOD VERSION:", reply)
}
