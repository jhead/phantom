package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/mdlayher/arp"
	"github.com/mdlayher/ethernet"
	"github.com/mdlayher/raw"
	"github.com/miekg/dns"
)

type sourceInterface struct {
	iface net.Interface
	ip    net.IP
	net   net.IPNet
}

type foundDevice struct {
	addr net.HardwareAddr
	ip   net.IP
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

func main() {
	source, err := getPreferredInterface()
	if err != nil {
		panic(err)
	}

	fmt.Println(source)

	ctx, stopScanning := context.WithCancel(context.Background())

	go arpEntireSubnet(ctx, *source)

	switchAddr, err := detectNintendoARP(*source)
	if err != nil {
		panic(err)
	}

	stopScanning()

	gatewayMac, err := net.ParseMAC("dc:7f:a4:0a:56:6d")
	if err != nil {
		panic(err)
	}

	gatewayIP := net.ParseIP("192.168.1.254") // todo

	go interceptDNS(switchAddr.addr, *source, gatewayMac)

	spoofGateway(*source, switchAddr.addr, gatewayIP)
}

func interceptDNS(targetMac net.HardwareAddr, source sourceInterface, gatewayMac net.HardwareAddr) {
	udpClient, err := raw.ListenPacket(&source.iface, uint16(ethernet.EtherTypeIPv4), &raw.Config{})
	if err != nil {
		panic(err)
	}

	if err := udpClient.SetPromiscuous(true); err != nil {
		panic(err)
	}

	fmt.Println("Intercepting DNS")

	buffer := make([]byte, 1500)
	for {
		read, _, err := udpClient.ReadFrom(buffer)
		if err != nil {
			fmt.Println(err)
			continue
		}

		readBuffer := buffer[:read]

		if read < 12 {
			// too short
			continue
		} else if !bytes.Equal(readBuffer[6:12], targetMac) {
			continue
		} else if bytes.Contains(readBuffer[12:], []byte{0x6d, 0x63, 0x6f, 0x04, 0x6c, 0x62, 0x73, 0x67, 0x03, 0x6e, 0x65, 0x74, 0x00}) {
			fmt.Println("DETECTED LBSG")

			dnsReq := dns.Msg{}
			err := dnsReq.Unpack(readBuffer[42:])
			fmt.Println(err)
			fmt.Println(dnsReq)

			sourcePortBytes := readBuffer[34:36]
			sourcePort := binary.BigEndian.Uint16(sourcePortBytes)

			rr, err := dns.NewRR(fmt.Sprintf("%s 3600 IN A %s", "mco.lbsg.net.", "104.219.3.4"))
			if err != nil {
				panic(err)
			}

			dnsReply := dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:       dnsReq.Id,
					Response: true,
					Opcode:   dns.OpcodeQuery,
					Rcode:    0,
				},
				Question: dnsReq.Question,
				Answer:   []dns.RR{rr},
			}

			replyBuffer, err := dnsReply.Pack()
			if err != nil {
				panic(err)
			}

			ipLayer := &layers.IPv4{
				Version:  4,
				SrcIP:    net.IP{192, 168, 1, 254},
				DstIP:    net.IP{192, 168, 1, 67},
				TTL:      64,
				Protocol: layers.IPProtocolUDP,
			}

			udpLayer := &layers.UDP{
				SrcPort: 53,
				DstPort: layers.UDPPort(sourcePort),
			}

			udpLayer.SetNetworkLayerForChecksum(ipLayer)

			buf := gopacket.NewSerializeBuffer()
			opts := gopacket.SerializeOptions{
				FixLengths:       true,
				ComputeChecksums: true,
			}

			err = gopacket.SerializeLayers(
				buf,
				opts,
				ipLayer,
				udpLayer,
				gopacket.Payload(replyBuffer),
			)

			if err != nil {
				panic(err)
			}

			replyFrame := ethernet.Frame{
				Destination: targetMac,
				Source:      source.iface.HardwareAddr,
				EtherType:   ethernet.EtherTypeIPv4,
				Payload:     buf.Bytes(),
			}

			fmt.Println(replyFrame)

			replyFrameBuffer, err := replyFrame.MarshalBinary()
			if err != nil {
				panic(err)
			}

			if _, err = udpClient.WriteTo(replyFrameBuffer, nil); err != nil {
				panic(err)
			}

			continue
		}

		frame := ethernet.Frame{}
		err = frame.UnmarshalBinary(readBuffer)
		if err != nil {
			panic(err)
		}

		frame.Source = source.iface.HardwareAddr
		frame.Destination = gatewayMac

		newFrameBuffer, err := frame.MarshalBinary()
		if err != nil {
			panic(err)
		}

		_, err = udpClient.WriteTo(newFrameBuffer, nil)
		if err != nil {
			fmt.Println(err)
		}
		// fmt.Printf("Forwarded %d -> %d bytes from %v\n", read, len(newFrameBuffer), addr)
	}
}

func spoofGateway(source sourceInterface, targetAddr net.HardwareAddr, gatewayIP net.IP) {
	arpClient, err := arp.Dial(&source.iface)
	if err != nil {
		panic(err)
	}

	// (op Operation, srcHW net.HardwareAddr, srcIP net.IP, dstHW net.HardwareAddr, dstIP net.IP)
	packet, err := arp.NewPacket(
		arp.OperationReply,
		source.iface.HardwareAddr,
		gatewayIP,
		source.iface.HardwareAddr,
		gatewayIP,
	)

	if err != nil {
		panic(err)
	}

	for {
		arpClient.WriteTo(packet, targetAddr)
		time.Sleep(time.Second)
	}
}

func arpEntireSubnet(ctx context.Context, source sourceInterface) {
	ips := ipsFromCIDR(source.net)
	arpClient, err := arp.Dial(&source.iface)
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

func detectNintendoARP(source sourceInterface) (*foundDevice, error) {
	arpClient, err := arp.Dial(&source.iface)
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
			fmt.Println(packet)
			if isNintendo(packet.SenderHardwareAddr) {
				fmt.Printf("Found %v\n", packet.SenderHardwareAddr)
				return &foundDevice{packet.SenderHardwareAddr, packet.SenderIP}, nil
			}
		}
	}
}

// Get preferred outbound ip of this machine
func getPreferredAddr() (net.IP, error) {
	// Doesn't actually create a connection, just prepares one
	conn, err := net.Dial("udp", "1.1.1.1:53")
	if err != nil {
		return nil, err
	}

	defer conn.Close()

	// The OS will automatically use the preferred IP
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}

func getPreferredInterface() (*sourceInterface, error) {
	addr, err := getPreferredAddr()
	if err != nil {
		return nil, err
	}

	fmt.Println(addr)
	return getInterfaceFromIP(addr.To4())
}

func getInterfaceFromIP(targetIP net.IP) (*sourceInterface, error) {
	ifaces, _ := net.Interfaces()

	// Iterate over all interfaces on the system
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()

		// Iterate over addresses on the interface
		for _, addr := range addrs {
			// Parse the CIDR notation to get just the IP
			ip, network, err := net.ParseCIDR(addr.String())
			if err != nil {
				return nil, err
			}

			// Skip IPv6 addrs
			ipv4 := ip.To4()
			if ipv4 == nil {
				continue
			}

			// Check if this IP ends with the target IP
			if bytes.HasSuffix(ipv4, targetIP) {
				return &sourceInterface{
					iface,
					ipv4,
					*network,
				}, nil
			}
		}
	}

	return nil, nil
}

func ipsFromCIDR(network net.IPNet) []net.IP {
	networkIP := network.IP

	var ips []net.IP
	for ip := networkIP.Mask(network.Mask); network.Contains(ip); inc(ip) {
		nextIP := make(net.IP, len(ip))
		copy(nextIP, ip)
		ips = append(ips, nextIP)
	}

	// Remove network address and broadcast address
	lenIPs := len(ips)

	// For a /32
	if lenIPs < 2 {
		return ips
	}

	return ips[1 : lenIPs-1]
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
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
