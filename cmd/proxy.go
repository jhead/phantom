package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jhead/phantom/internal/proxy"
)

var bindAddressString string
var serverAddressString string
var bindPortInt uint16

func main() {
	bindArg := flag.String("bind", "0.0.0.0", "IP address to listen on, port is randomized by default - see: bind_port")
	bindPortArg := flag.Int("bind_port", 0, "Port to listen on - 0 means a random port will be used (default 0)")
	serverArg := flag.String("server", "", "Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)")
	timeoutArg := flag.Int("timeout", 60, "Seconds to wait before cleaning up a disconnected client")

	flag.Usage = usage
	flag.Parse()

	if *serverArg == "" {
		flag.Usage()
		return
	}

	bindAddressString = *bindArg
	serverAddressString = *serverArg
	idleTimeout := time.Duration(*timeoutArg) * time.Second
	bindPortInt = uint16(*bindPortArg)

	fmt.Printf("Starting up with remote server IP: %s\n", serverAddressString)

	proxyServer, err := proxy.New(proxy.ProxyPrefs{
		bindAddressString,
		bindPortInt,
		serverAddressString,
		idleTimeout,
	})

	if err != nil {
		fmt.Printf("Failed to init server: %s\n", err)
		return
	}

	if err := proxyServer.Start(); err != nil {
		fmt.Printf("Failed to start server: %s\n", err)
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] -server <server-ip>\n\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
}
