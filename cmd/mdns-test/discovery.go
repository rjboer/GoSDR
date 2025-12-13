package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rjboer/GoSDR/internal/mdns"
)

func main() {
	timeout := flag.Int("timeout", 5, "Timeout in seconds")
	flag.Parse()

	fmt.Println("===============================================================")
	fmt.Println(" mDNS / DNS-SD Discovery Test")
	fmt.Println("===============================================================")
	fmt.Printf(" Service : _iio._tcp.local\n")
	fmt.Printf(" Timeout : %d seconds\n", *timeout)
	fmt.Println("---------------------------------------------------------------")

	start := time.Now()
	hosts, err := mdns.DiscoverIIOD(*timeout)
	duration := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Discovery error: %v\n", err)
		os.Exit(1)
	}

	if len(hosts) == 0 {
		fmt.Printf("No devices found (%s)\n", duration.Truncate(time.Millisecond))
		return
	}

	fmt.Printf("Discovered %d device(s) in %s\n",
		len(hosts), duration.Truncate(time.Millisecond))
	fmt.Println("===============================================================")

	for i, h := range hosts {
		fmt.Printf(" Device #%d\n", i+1)
		fmt.Println("---------------------------------------------------------------")
		fmt.Printf(" Instance : %s\n", h.Instance)
		fmt.Printf(" Hostname : %s\n", h.Hostname)
		fmt.Printf(" Port     : %d\n", h.Port)

		fmt.Println(" Addresses:")
		if len(h.Addresses) == 0 {
			fmt.Println("   <none>")
		} else {
			for _, ip := range h.Addresses {
				fmt.Printf("   - %s\n", ip.String())
			}
		}

		fmt.Println(" TXT Records:")
		if len(h.TXT) == 0 {
			fmt.Println("   <none>")
		} else {
			for _, txt := range h.TXT {
				fmt.Printf("   - %s\n", txt)
			}
		}

		// Derived connection hints
		fmt.Println(" Connection hints:")
		for _, ip := range h.Addresses {
			if ip.To4() != nil {
				fmt.Printf("   - tcp://%s:%d\n", ip.String(), h.Port)
			} else {
				fmt.Printf("   - tcp://[%s]:%d\n", ip.String(), h.Port)
			}
		}

		fmt.Println("===============================================================")
	}
}
