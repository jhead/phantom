package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/jhead/phantom/internal/switch/findswitch"
	"github.com/jhead/phantom/internal/switch/mitm"
	"github.com/jhead/phantom/internal/switch/netutils"
	"github.com/mdlayher/ethernet"
	"github.com/mdlayher/raw"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

func main() {
	// go interceptDNS(switchAddr.addr, *source, gatewayMac)
	// spoofGateway(*source, switchAddr.addr, gatewayIP)
	iface, err := netutils.GetPreferredInterface()
	if err != nil {
		panic(err)
	}

	gatewayIP, gatewayMAC, err := netutils.GetDefaultGateway()
	if err != nil {
		panic(err)
	}

	switchDevice, err := findswitch.FindNintendoSwitch()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recvPackets := make(chan mitm.Packet)
	dnsChan := make(chan []byte)

	// Creates a raw socket capable of receiving all traffic on this interface
	etherClient, err := raw.ListenPacket(&iface.Interface, uint16(ethernet.EtherTypeIPv4), &raw.Config{})
	if err != nil {
		panic(err)
	}

	filters := packetFilters(*switchDevice, dnsChan)

	go mitm.ForwardAll(ctx, etherClient, recvPackets, iface.Interface, gatewayMAC)
	go mitm.Sniff(ctx, etherClient, filters, recvPackets)
	go mitm.SpoofARP(ctx, *iface, switchDevice.MAC, gatewayIP)

	injectDNSReplies(ctx, etherClient, iface.Interface, gatewayIP, *switchDevice, dnsChan)
}

func packetFilters(switchDevice findswitch.NintendoSwitch, dnsChan chan []byte) []mitm.PacketFilter {
	shortFilter := func(packet []byte) bool {
		return len(packet) > 12
	}

	sourceMatchesSwitch := func(packet []byte) bool {
		return bytes.Equal(packet[6:12], switchDevice.MAC)
	}

	interceptDNS := func(packet []byte) bool {
		if shouldInterceptPacket(packet) {
			dnsChan <- packet
			return false
		}

		return true
	}

	return []mitm.PacketFilter{
		shortFilter,
		sourceMatchesSwitch,
		interceptDNS,
	}
}

func shouldInterceptPacket(packet []byte) bool {
	// todo: better
	lbsgBytes := []byte{0x6d, 0x63, 0x6f, 0x04, 0x6c, 0x62, 0x73, 0x67, 0x03, 0x6e, 0x65, 0x74, 0x00}
	return bytes.Contains(packet[12:], lbsgBytes)
}

func injectDNSReplies(
	ctx context.Context,
	etherClient *raw.Conn,
	iface net.Interface,
	gatewayIP net.IP,
	switchDevice findswitch.NintendoSwitch,
	dnsChan chan []byte,
) error {
	for packetBuffer := <-dnsChan; ctx.Done() != nil; {
		fmt.Println("DETECTED LBSG")

		dnsReq := dns.Msg{}
		err := dnsReq.Unpack(packetBuffer[42:])
		fmt.Println(err)
		fmt.Println(dnsReq)

		dnsReply, err := createDNSReply(dnsReq, "104.219.3.4")
		if err != nil {
			return err
		}

		replyFrameBuffer, err := createDNSReplyFrame(iface, packetBuffer, *dnsReply, gatewayIP, switchDevice)
		if err != nil {
			return err
		}

		if _, err = etherClient.WriteTo(replyFrameBuffer, nil); err != nil {
			return err
		}
	}

	return nil
}

func createDNSReply(request dns.Msg, serverIP string) (*dns.Msg, error) {
	if len(request.Question) != 1 {
		return nil, errors.Errorf("DNS request must have 1 question")
	}

	hostname := request.Question[0].Name
	ttl := 15

	rrString := fmt.Sprintf("%s %d IN A %s", hostname, ttl, serverIP)
	fmt.Println(rrString)

	rr, err := dns.NewRR(rrString)
	if err != nil {
		return nil, err
	}

	dnsReply := dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:       request.Id,
			Response: true,
			Opcode:   dns.OpcodeQuery,
			Rcode:    0,
		},
		Question: request.Question,
		Answer:   []dns.RR{rr},
	}

	return &dnsReply, nil
}

func createDNSReplyFrame(
	iface net.Interface,
	originalPacket []byte,
	dnsReply dns.Msg,
	spoofedIP net.IP,
	switchDevice findswitch.NintendoSwitch,
) ([]byte, error) {
	// Read the port that the DNS request came from, so that we can reply to it
	sourcePortBytes := originalPacket[34:36]
	sourcePort := binary.BigEndian.Uint16(sourcePortBytes)

	// Turn the DNS reply into bytes
	dnsReplyBuffer, err := dnsReply.Pack()
	if err != nil {
		return nil, err
	}

	// Built the IPv4 layer
	ipLayer := &layers.IPv4{
		Version:  4,
		SrcIP:    spoofedIP, // todo: read DNS server IP from packet using gopacket
		DstIP:    switchDevice.IP,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
	}

	// Build the UDP layer
	udpLayer := &layers.UDP{
		SrcPort: 53, // todo: match to request?
		DstPort: layers.UDPPort(sourcePort),
	}

	// Include a checksum or else the packet will be dropped
	udpLayer.SetNetworkLayerForChecksum(ipLayer)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		// Compute these fields on the fly
		FixLengths:       true,
		ComputeChecksums: true,
	}

	// Serialize!
	if err = gopacket.SerializeLayers(
		buf,
		opts,
		ipLayer,
		udpLayer,
		gopacket.Payload(dnsReplyBuffer),
	); err != nil {
		return nil, err
	}

	// Build the ethernet frame
	// todo: replace with gopacket ethernet layer
	replyFrame := ethernet.Frame{
		Destination: switchDevice.MAC,
		Source:      iface.HardwareAddr,
		EtherType:   ethernet.EtherTypeIPv4,
		Payload:     buf.Bytes(),
	}

	// Turn the frame into bytes
	return replyFrame.MarshalBinary()
}
