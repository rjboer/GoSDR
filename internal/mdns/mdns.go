package mdns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// Host represents a discovered IIOD-capable device
type Host struct {
	Instance  string // Advertised name: "iiod on pluto"
	Hostname  string // DNS hostname: "pluto.local."
	Addresses []net.IP
	Port      int
	TXT       []string
}

// DiscoverIIOD performs a blocking mDNS browse for _iio._tcp.local services.
// It returns cleaned and deduplicated host entries.
func DiscoverIIOD(timeoutSeconds int) ([]Host, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("resolver error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	entries := make(chan *zeroconf.ServiceEntry)
	resultMap := make(map[string]Host)

	// Consumer goroutine
	done := make(chan struct{})
	go func() {
		for {
			select {
			case e, ok := <-entries:
				if !ok {
					close(done)
					return
				}
				if e == nil {
					continue
				}

				// Consolidate IPs (both v4 and v6)
				addrs := make([]net.IP, 0, len(e.AddrIPv4)+len(e.AddrIPv6))
				addrs = append(addrs, e.AddrIPv4...)
				addrs = append(addrs, e.AddrIPv6...)

				// Pick a stable key
				key := fmt.Sprintf("%s|%d", e.HostName, e.Port)

				resultMap[key] = Host{
					Instance:  cleanInstance(e.Instance),
					Hostname:  e.HostName,
					Addresses: addrs,
					Port:      e.Port,
					TXT:       append([]string{}, e.Text...),
				}

			case <-ctx.Done():
				close(done)
				return
			}
		}
	}()

	// Start browsing
	if err := resolver.Browse(ctx, "_iio._tcp", "local.", entries); err != nil {
		return nil, fmt.Errorf("browse error: %w", err)
	}

	<-done // wait for results

	// Convert map -> slice
	out := make([]Host, 0, len(resultMap))
	for _, h := range resultMap {
		out = append(out, h)
	}

	return out, nil
}

// cleanInstance removes Zeroconf escape sequences: "\ " => " "
func cleanInstance(s string) string {
	return strings.ReplaceAll(s, `\ `, " ")
}
