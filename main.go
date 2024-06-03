package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/grandcat/zeroconf"
	"golang.design/x/clipboard"
)

const PORT = 12345
const ServiceName = "Network clipboard"
const BufferSize = 1024

type Client struct {
	IP   net.IP
	Port int
	Name string
}

var clients []Client = make([]Client, 0)

func main() {
	err := clipboard.Init()
	if err != nil {
		panic(err)
	}

	go registerService()
	var mu sync.Mutex
	go browseServices(&clients, &mu)

	go func() {
		ch := clipboard.Watch(context.TODO(), clipboard.FmtText)

		for data := range ch {
			fmt.Println(string(data))
			sendData(data)
		}
	}()

	go listenForTcp()

	select {}

}

func registerService() {
	server, err := zeroconf.Register(ServiceName, "_workstation._udp", "local.", PORT, []string{""}, nil)
	if err != nil {
		panic(err)
	}
	defer server.Shutdown()
	defer log.Println("Shutting down.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

func browseServices(clients *[]Client, mu *sync.Mutex) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatalf("Failed to initialize resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func(entries <-chan *zeroconf.ServiceEntry) {
		for entry := range entries {
			client := Client{
				IP:   entry.AddrIPv4[0],
				Port: entry.Port,
				Name: entry.Service,
			}

			mu.Lock()
			if !clientExists(clients, client) {
				*clients = append(*clients, client)
			}
			mu.Unlock()
		}
	}(entries)

	for {
		err = resolver.Browse(context.TODO(), "_workstation._udp", "local.", entries)
		if err != nil {
			log.Fatalf("Failed to browse: %v", err)
		}
		printClients(clients)
		time.Sleep(time.Second * 10)
	}
}

func printClients(clients *[]Client) {
	for _, client := range *clients {
		fmt.Printf("- %s(%s:%d)\n", client.Name, client.IP.String(), client.Port)
	}
}

func clientExists(clients *[]Client, client Client) bool {
	for _, c := range *clients {
		if c.IP.Equal(client.IP) && c.Port == client.Port && c.Name == client.Name {
			return true
		}
	}
	return false
}

func sendData(data []byte) {

	fmt.Printf("Sending: %s", string(data))

	for i := range clients {
		var client = &clients[i]
		serverAddr := fmt.Sprintf("%s:%d", client.IP, client.Port)

		resolvedAddr, err := net.ResolveTCPAddr("tcp4", serverAddr)
		if err != nil {
			fmt.Println("Error resolving address:", err)

			continue
		}

		conn, err := net.DialTCP("tcp4", nil, resolvedAddr)
		if err != nil {
			fmt.Println("Error dialing:", err)
			continue
		}

		_, err = conn.Write(data)

		if err != nil {
			println("Write to server failed:", err.Error())
			os.Exit(1)
		}

		println("write to server = ", string(data))

		reply := make([]byte, 1024)

		_, err = conn.Read(reply)
		if err != nil {
			println("Write to server failed:", err.Error())
			os.Exit(1)
		}

		println("reply from server=", string(reply))

		conn.Close()
	}

}

func getLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, i := range interfaces {
		addrs, err := i.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("no IP address found")
}

func listenForUdp() {
	err := beeep.Notify("Network clipboard", "Listening for events", "")
	if err != nil {
		log.Fatalf("Notification error: %v", err)
	}

	localIP, err := getLocalIP()
	if err != nil {
		log.Fatalf("Failed to get local IP: %v", err)
	}
	serverAddr := fmt.Sprintf("%s:%d", localIP, PORT)
	fmt.Printf("Listening for data on %s\n", serverAddr)

	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		log.Fatalf("Error resolving address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Error creating UDP server: %v", err)
	}

	buffer := make([]byte, BufferSize)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer[0:])
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		conn.WriteToUDP([]byte("Hello UDP Client\n"), addr)

		receivedData := string(buffer[:n])
		prefix := "Received from UDP: "
		dataWithPrefix := prefix + receivedData
		dataWithPrefixBytes := []byte(dataWithPrefix)

		fmt.Printf("\nReceived \"%s\" from %s\n", receivedData, clientAddr)
		clipboard.Write(clipboard.FmtText, dataWithPrefixBytes)
		if err != nil {
			log.Printf("Error writing to clipboard: %v", err)
		}
	}
}

func listenForTcp() {
	addr := "localhost:12345"
	l, err := net.Listen("tcp4", addr)
	if err != nil {
		panic(err)
	}
	defer l.Close()

	fmt.Printf("Listening on localhost:12345\n")

	for {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		go func(conn net.Conn) {
			buf := make([]byte, 1024)
			len, err := conn.Read(buf)
			if err != nil {
				fmt.Printf("Error reading: %#v\n", err)
				return
			}
			fmt.Printf("Message received: %s\n", string(buf[:len]))

			conn.Write([]byte("Message received.\n"))
			clipboard.Write(clipboard.FmtText, buf[:len])
			if err != nil {
				log.Printf("Error writing to clipboard: %v", err)
			}
			conn.Close()
		}(conn)
	}
}
