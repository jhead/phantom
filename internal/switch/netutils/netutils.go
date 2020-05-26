package netutils

import (
	"bytes"
	"fmt"
	"net"
)

type Interface struct {
	Interface net.Interface
	IP        net.IP
	Network   net.IPNet
}

// Get preferred outbound ip of this machine
func GetPreferredAddr() (net.IP, error) {
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

func GetPreferredInterface() (*Interface, error) {
	addr, err := GetPreferredAddr()
	if err != nil {
		return nil, err
	}

	fmt.Println(addr)
	return GetInterfaceFromIP(addr.To4())
}

func GetInterfaceFromIP(targetIP net.IP) (*Interface, error) {
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
				return &Interface{
					iface,
					ipv4,
					*network,
				}, nil
			}
		}
	}

	return nil, nil
}

func AllNetworkIPs(network net.IPNet) []net.IP {
	networkIP := network.IP

	var ips []net.IP
	for ip := networkIP.Mask(network.Mask); network.Contains(ip); incrementIP(ip) {
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

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// todo
func GetDefaultGateway() (net.IP, net.HardwareAddr, error) {
	gatewayMac, err := net.ParseMAC("dc:7f:a4:0a:56:6d")
	if err != nil {
		return nil, nil, err
	}

	gatewayIP := net.ParseIP("192.168.1.254")

	return gatewayIP, gatewayMac, nil
}
