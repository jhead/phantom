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
	// Required
	serverArg := flag.String("server", "", "Required: Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)")

	// Optional
	bindArg := flag.String("bind", "0.0.0.0", "Optional: IP address to listen on. Defaults to all interfaces.")
	bindPortArg := flag.Int("bind_port", 0, "Optional: Port to listen on. Defaults to 0, which selects a random port.\nNote that phantom always binds to port 19132 as well, so both ports need to be open.")
	timeoutArg := flag.Int("timeout", 60, "Optional: Seconds to wait before cleaning up a disconnected client")

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
