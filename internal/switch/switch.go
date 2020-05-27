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

type ServerListHijacker struct {
	iface           netutils.Interface
	gatewayIP       net.IP
	gatewayMAC      net.HardwareAddr
	etherClient     *raw.Conn
	device          findswitch.NintendoSwitch
	recvPacketsChan chan mitm.Packet
	dnsChan         chan knownDNSRequest
}

type knownDNSRequest struct {
	req          dns.Msg
	packetBuffer []byte
}

var knownServers = map[string]bool{
	"geo.hivebedrock.network.": true,
	"hivebedrock.network.":     true,
	"mco.mineplex.com.":        true,
	"play.inpvp.net.":          true,
	"mco.lbsg.net.":            true,
	"mco.cubecraft.net.":       true,
}

var port53Bytes = []byte{0x00, 0x35}

func main() {
	inst, _ := New()
	inst.Start()
}

func New() (*ServerListHijacker, error) {
	iface, err := netutils.GetPreferredInterface()
	if err != nil {
		return nil, err
	}

	gatewayIP, gatewayMAC, err := netutils.GetDefaultGateway()
	if err != nil {
		return nil, err
	}

	switchDevice, err := findswitch.FindNintendoSwitch()

	recvPacketsChan := make(chan mitm.Packet)
	dnsChan := make(chan knownDNSRequest)

	// Creates a raw socket capable of receiving all traffic on this interface
	etherClient, err := raw.ListenPacket(&iface.Interface, uint16(ethernet.EtherTypeIPv4), &raw.Config{})
	if err != nil {
		return nil, err
	}

	return &ServerListHijacker{
		*iface,
		gatewayIP,
		gatewayMAC,
		etherClient,
		*switchDevice,
		recvPacketsChan,
		dnsChan,
	}, nil
}

func (inst ServerListHijacker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Starting packet sniffing and MITM")

	filters := inst.packetFilters()

	go mitm.ForwardAll(
		ctx,
		inst.etherClient,
		inst.recvPacketsChan,
		inst.iface.Interface,
		inst.gatewayMAC,
	)

	go mitm.Sniff(ctx, inst.etherClient, filters, inst.recvPacketsChan)
	go mitm.SpoofARP(ctx, inst.iface, inst.device.MAC, inst.gatewayIP)

	inst.injectDNSReplies(ctx)
}

func (inst ServerListHijacker) packetFilters() []mitm.PacketFilter {
	shortFilter := func(packet []byte) bool {
		return len(packet) > 42
	}

	sourceMatchesSwitch := func(packet []byte) bool {
		return bytes.Equal(packet[6:12], inst.device.MAC)
	}

	interceptDNS := func(packet []byte) bool {
		shouldIntercept, dnsReq := shouldInterceptPacket(packet)
		if shouldIntercept {
			fmt.Printf("Sending DNS req to chan\n")
			inst.dnsChan <- knownDNSRequest{*dnsReq, packet}
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

func shouldInterceptPacket(packetBuffer []byte) (bool, *dns.Msg) {
	isPort53 := bytes.Equal(packetBuffer[36:38], port53Bytes)
	if !isPort53 {
		return false, nil
	}

	dnsReq := dns.Msg{}
	if err := dnsReq.Unpack(packetBuffer[42:]); err != nil {
		fmt.Println(err)
		return false, nil
	}

	if len(dnsReq.Question) != 1 {
		fmt.Println("DNS request does not have 1 question")
		return false, nil
	}

	dnsLookupHostname := dnsReq.Question[0].Name
	fmt.Println(dnsLookupHostname)
	if _, isKnownServer := knownServers[dnsLookupHostname]; !isKnownServer {
		return false, nil
	}

	return true, &dnsReq
}

func (inst ServerListHijacker) injectDNSReplies(ctx context.Context) error {
	for ctx.Done() != nil {
		dnsReq := <-inst.dnsChan
		fmt.Printf("Overriding server request: %v", dnsReq.req)

		dnsReply, err := createDNSReply(dnsReq.req, "104.219.3.4")
		if err != nil {
			return err
		}

		replyFrameBuffer, err := inst.createDNSReplyFrame(dnsReq.packetBuffer, *dnsReply)
		if err != nil {
			return err
		}

		if _, err = inst.etherClient.WriteTo(replyFrameBuffer, nil); err != nil {
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

	fmt.Println(dnsReply)
	return &dnsReply, nil
}

func (inst ServerListHijacker) createDNSReplyFrame(
	originalPacket []byte,
	dnsReply dns.Msg,
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
		SrcIP:    inst.gatewayIP, // todo: read DNS server IP from packet using gopacket
		DstIP:    inst.device.IP,
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
		Destination: inst.device.MAC,
		Source:      inst.iface.Interface.HardwareAddr,
		EtherType:   ethernet.EtherTypeIPv4,
		Payload:     buf.Bytes(),
	}

	// Turn the frame into bytes
	return replyFrame.MarshalBinary()
}
