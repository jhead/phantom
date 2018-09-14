package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/mostlygeek/arp"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// Extremely experimental and messy attempt at advertising remote
// servers over LAN by responding to MCPE broadcast packets but
// spoofing the UDP source address in the replies.
//
// Requires sudo/admin due to raw packet access.

const SPOOFED = "104.219.6.162"

func main() {
	udp, err := net.ListenPacket("udp", "192.168.1.255:19132")
	if err != nil {
		panic(err)
	}

	defer udp.Close()

	buffer := make([]byte, 1024)

	for {
		read, source, _ := udp.ReadFrom(buffer)
		buffer = buffer[:read]

		fmt.Println(source)
		fmt.Println(buffer)

		id := buffer[0]
		pingID := buffer[1:9]
		magic := buffer[9:25]
		fmt.Printf("id: %d pingID: %v magic: %v\n", id, pingID, magic)

		localAddr, _ := net.ResolveIPAddr("ip4", "192.168.1.71")
		remoteAddr, _ := net.ResolveIPAddr("ip4", "192.168.1.69")

		fmt.Println(localAddr)
		fmt.Println(remoteAddr)

		localMAC := arp.Search(localAddr.IP.String())
		fmt.Println(localMAC)
		remoteMAC := arp.Search(remoteAddr.IP.String())
		fmt.Println(remoteMAC)

		remoteCon, err := net.DialIP("ip4:4", localAddr, remoteAddr)

		if err != nil {
			panic(err)
		}

		var outBuffer bytes.Buffer
		serverName := fmt.Sprintf("MCPE;Spoof %s;2 7;0.11.0;0;20", SPOOFED)
		outBuffer.WriteByte(0x1c)
		outBuffer.Write(pingID)
		outBuffer.Write(pingID)
		outBuffer.Write(magic)

		serverNameLen := uint16(len(serverName))
		stringBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(stringBuf, serverNameLen)

		outBuffer.Write(stringBuf)
		outBuffer.WriteString(serverName)

		outBufferBytes := outBuffer.Bytes()
		fmt.Println(outBufferBytes)

		sourceAddr, err := net.ResolveUDPAddr("udp", source.String())
		if err != nil {
			panic(err)
		}

		sourceMac, _ := net.ParseMAC(localMAC)
		destMac, _ := net.ParseMAC(remoteMAC)

		spoofedAddr, _ := net.ResolveIPAddr("ip4", SPOOFED)
		// spoofedAddr, _ := net.ResolveIPAddr("ip4", "192.168.1.72")

		packet, err := createSerializedUDPFrame(udpFrameOptions{
			sourceIP:     spoofedAddr.IP,
			destIP:       remoteAddr.IP,
			sourcePort:   19132,
			sourceMac:    sourceMac,
			destMac:      destMac,
			destPort:     uint16(sourceAddr.Port),
			isIPv6:       false,
			payloadBytes: outBufferBytes,
		})

		if err != nil {
			panic(err)
		}

		handle, _ := pcap.OpenLive("en0", 1024, false, pcap.BlockForever)
		defer handle.Close()

		if err = handle.WritePacketData(packet); err != nil {
			panic(err)
		}

		remoteCon.Close()
	}
}

type udpFrameOptions struct {
	sourceIP, destIP     net.IP
	sourcePort, destPort uint16
	sourceMac, destMac   net.HardwareAddr
	isIPv6               bool
	payloadBytes         []byte
}

type serializableNetworkLayer interface {
	gopacket.NetworkLayer
	gopacket.SerializableLayer
}

func createSerializedUDPFrame(opts udpFrameOptions) ([]byte, error) {

	buf := gopacket.NewSerializeBuffer()
	serializeOpts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	ethernetType := layers.EthernetTypeIPv4
	if opts.isIPv6 {
		ethernetType = layers.EthernetTypeIPv6
	}
	eth := &layers.Ethernet{
		SrcMAC:       opts.sourceMac,
		DstMAC:       opts.destMac,
		EthernetType: ethernetType,
	}
	var ip serializableNetworkLayer
	if !opts.isIPv6 {
		ip = &layers.IPv4{
			SrcIP:    opts.sourceIP,
			DstIP:    opts.destIP,
			Protocol: layers.IPProtocolUDP,
			Version:  4,
			TTL:      32,
		}
	} else {
		ip = &layers.IPv6{
			SrcIP:      opts.sourceIP,
			DstIP:      opts.destIP,
			NextHeader: layers.IPProtocolUDP,
			Version:    6,
			HopLimit:   32,
		}
		ip.LayerType()
	}

	udp := &layers.UDP{
		SrcPort: layers.UDPPort(opts.sourcePort),
		DstPort: layers.UDPPort(opts.destPort),
		// we configured "Length" and "Checksum" to be set for us
	}
	udp.SetNetworkLayerForChecksum(ip)
	err := gopacket.SerializeLayers(buf, serializeOpts, eth, ip, udp, gopacket.Payload(opts.payloadBytes))
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
