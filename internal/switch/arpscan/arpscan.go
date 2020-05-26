package arpscan

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jhead/phantom/internal/switch/netutils"
	"github.com/mdlayher/arp"
)

func Scan(ctx context.Context, network net.IPNet, iface *net.Interface) {
	ips := netutils.AllNetworkIPs(network)
	arpClient, err := arp.Dial(iface)
	if err != nil {
		panic(err)
	}

	// Loop until cancelled
	for {
		// Scan the entire range using ARP requests
		// Replies are collected in another goroutine
		for _, ip := range ips {
			// Stop scanning if cancelled
			if ctx.Err() != nil {
				fmt.Println("Scan cancelled")
				return
			}

			fmt.Printf("ARPing %v\n", ip)

			arpClient.Request(ip)
			if err != nil {
				panic(err)
			}

			// Sending too fast will cause them to drop
			time.Sleep(25 * time.Millisecond)
		}
	}
}
