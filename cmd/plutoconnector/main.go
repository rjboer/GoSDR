package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/rjboer/GoSDR/iiod"
)

var dial = iiod.Dial

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Getenv); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("gosdr", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	defaultAddr := strings.TrimSpace(getenv("IIOD_ADDR"))
	if defaultAddr == "" {
		defaultAddr = "192.168.2.1:30431"
	}

	addr := fs.String("iiod-addr", defaultAddr, "IIOD host:port address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	c, err := dial(*addr)
	if err != nil {
		return fmt.Errorf("failed to dial IIOD: %w", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			log.Printf("failed to close IIOD client: %v", err)
		}
	}()

	info, err := c.GetContextInfo()
	if err != nil {
		return fmt.Errorf("failed to get context info: %w", err)
	}

	_, err = fmt.Fprintf(out, "IIOD VERSION: %d.%d %s\n", info.Major, info.Minor, info.Description)
	return err
}
