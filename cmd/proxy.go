package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/jhead/bedrock-proxy/internal/proto"
)

const MaxMTU = 1440
const RemoteServerIP = "104.219.6.162:19132"

var bindAddressString string
var serverAddressString string
var proxyServer *net.UDPConn
var connectionMap = map[string]*net.UDPConn{}
var broadcastPackets = make(chan Packet)

type Packet struct {
	server net.PacketConn
	source net.Addr
	data   []byte
}

func main() {
	bindArg := flag.String("bind", "0.0.0.0:19132", "Bind address and port")
	serverArg := flag.String("server", RemoteServerIP, "Real Bedrock/MCPE server IP and port")
	flag.Parse()

	bindAddressString = *bindArg
	serverAddressString = *serverArg

	fmt.Printf("Remote server: %s\n", serverAddressString)

	if err := startProxyServer(); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}

func startProxyServer() error {
	fmt.Printf("Binding UDP server to: %v\n", bindAddressString)

	bindAddr, err := net.ResolveUDPAddr("udp", bindAddressString)
	if err != nil {
		return err
	}

	proxyServer, err = net.ListenUDP("udp", bindAddr)
	if err != nil {
		return err
	}

	defer proxyServer.Close()
	go handleBroadcastPackets()

	packetBuffer := make([]byte, MaxMTU)

	for {
		err := processDataFromClients(proxyServer, packetBuffer)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func processDataFromClients(proxyServer *net.UDPConn, packetBuffer []byte) error {
	read, source, _ := proxyServer.ReadFrom(packetBuffer)
	data := packetBuffer[:read]

	// fmt.Printf("Got packet from %s: %d, len: %d %d\n", source, packetBuffer[0], len(data))
	// fmt.Println(data)

	if data[0] == proto.UnconnectedRequestID {
		broadcastPackets <- Packet{proxyServer, source, data}
		return nil
	}

	conn := getConnection(source)

	_, err := conn.Write(data)
	// fmt.Printf("Wrote %d bytes to server\n", written)

	return err
}

func getConnection(source net.Addr) *net.UDPConn {
	key := source.String()

	if conn, ok := connectionMap[key]; ok {
		return conn
	}

	localAddr, _ := net.ResolveUDPAddr("udp", "192.168.1.71:0")
	remoteAddr, _ := net.ResolveUDPAddr("udp", RemoteServerIP)

	// fmt.Println(localAddr)
	// fmt.Println(remoteAddr)

	conn, err := net.DialUDP("udp", localAddr, remoteAddr)
	if err != nil {
		panic(err)
	}

	connectionMap[key] = conn
	go handleClientbound(conn, source)

	return conn
}

func handleClientbound(conn *net.UDPConn, source net.Addr) {
	fmt.Printf("Opening UDP connection to %v for new client!\n", source.String())
	buffer := make([]byte, MaxMTU)

	for {
		read, _, err := conn.ReadFrom(buffer)

		if err != nil {
			fmt.Println(err)
			break
		}

		// fmt.Printf("Writing %d bytes from server to client\n", read)
		data := buffer[:read]
		proxyServer.WriteTo(data, source)
	}
}

func handleBroadcastPackets() {
	fmt.Println("Starting ping handler")

	for {
		packet := <-broadcastPackets
		id := packet.data[1:9]
		magic := packet.data[9:25]

		fmt.Printf("Ping from %v!\n", packet.source)

		serverName := fmt.Sprintf("MCPE;Remote %s;2 7;0.11.0;0;20", RemoteServerIP)
		replyBuffer := proto.UnconnectedReply{id, magic, serverName}.Build()

		packet.server.WriteTo(replyBuffer.Bytes(), packet.source)
	}
}
