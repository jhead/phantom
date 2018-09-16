package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jhead/bedrock-proxy/internal/proxy"
)

var bindAddressString string
var serverAddressString string

func main() {
	bindArg := flag.String("bind", "0.0.0.0:19132", "Bind address and port")
	flag.Usage = usage
	flag.Parse()

	if len(os.Args) != 2 {
		usage()
		return
	}

	bindAddressString = *bindArg
	serverAddressString = os.Args[1]

	fmt.Printf("Starting up with remote server IP: %s\n", serverAddressString)

	proxyServer, err := proxy.New(bindAddressString, serverAddressString)
	if err != nil {
		fmt.Printf("Failed to init server: %s\n", err)
		return
	}

	if err := proxyServer.Start(); err != nil {
		fmt.Printf("Failed to start server: %s\n", err)
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s <server-ip>\n\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
}
