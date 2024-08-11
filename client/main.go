package main

import "flag"

func main() {
	var clientID, serverAddr string
	flag.StringVar(&clientID, "client_id", "", "client id")
	flag.StringVar(&serverAddr, "server_addr", "", "server address")
	flag.Parse()

	c := NewClient(clientID, serverAddr)
	c.Run()
}
