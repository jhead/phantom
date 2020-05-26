package findswitch

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/jhead/phantom/internal/switch/arpscan"
	"github.com/jhead/phantom/internal/switch/netutils"
	"github.com/mdlayher/arp"
)

type NintendoSwitch struct {
	MAC net.HardwareAddr
	IP  net.IP
}

var macPrefixes = map[string]bool{
	"0403D6": true,
	"342FBD": true,
	"48A5E7": true,
	"582F40": true,
	"5C521E": true,
	"606BFF": true,
	"64B5C6": true,
	"7048F7": true,
	"9458CB": true,
	"98415C": true,
	"98B6E9": true,
	"98E8FA": true,
	"A438CC": true,
	"B87826": true,
	"B88AEC": true,
	"D4F057": true,
	"DC68EB": true,
	"E0F6B5": true,
	"E8DA20": true,
	"ECC40D": true,
}

func FindNintendoSwitch() (*NintendoSwitch, error) {
	iface, err := netutils.GetPreferredInterface()
	if err != nil {
		return nil, err
	}

	ctx, stopScanning := context.WithCancel(context.Background())
	defer stopScanning()

	go arpscan.Scan(ctx, iface.Network, &iface.Interface)
	return detectNintendoARP(iface.Interface)
}

// todo: use a channel and support multiple devices
// todo: narrow down to the Switch, not just Nintendo devices

func detectNintendoARP(source net.Interface) (*NintendoSwitch, error) {
	arpClient, err := arp.Dial(&source)
	if err != nil {
		return nil, err
	}

	for {
		packet, _, err := arpClient.Read()
		if err != nil {
			return nil, err
		}

		if packet.Operation != arp.OperationReply {
			// Read only ARP replies
			continue
		}

		if packet != nil {
			if isNintendo(packet.SenderHardwareAddr) {
				fmt.Printf("Found %v\n", packet.SenderHardwareAddr)

				return &NintendoSwitch{
					packet.SenderHardwareAddr,
					packet.SenderIP,
				}, nil
			}
		}
	}
}

func isNintendo(addr net.HardwareAddr) bool {
	mac := strings.ToUpper(addr.String())
	mac = strings.ReplaceAll(mac, ":", "")
	mac = mac[:6]

	_, exists := macPrefixes[mac]
	return exists
}
