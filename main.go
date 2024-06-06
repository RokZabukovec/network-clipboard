package main

import (
	"bytes"
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

const PORT = 8888
const ServiceName = "Network clipboard"
const BufferSize = 1024

var ignoreNextChange bool

type Client struct {
	IP   net.IP
	Port int
	Name string
}

var clients = make([]Client, 0)

func main() {
	err := clipboard.Init()
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("Shutting down...")
		cancel()
		wg.Wait()
		os.Exit(0)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		registerService(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		browseServices(ctx, &clients)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		watchClipboard(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		listenForTcp(ctx)
	}()

	select {}
}

func registerService(ctx context.Context) {
	server, err := zeroconf.Register(ServiceName, "_workstation._tcp", "local.", PORT, []string{""}, nil)
	if err != nil {
		panic(err)
	}

	defer server.Shutdown()
	defer log.Println("Shutting down.")

	<-ctx.Done()
}

func browseServices(ctx context.Context, clients *[]Client) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatalf("Failed to initialize resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func(entries <-chan *zeroconf.ServiceEntry) {
		for {
			select {
			case entry := <-entries:
				client := Client{
					IP:   entry.AddrIPv4[0],
					Port: entry.Port,
					Name: entry.Service,
				}

				if !clientExists(clients, client) {
					*clients = append(*clients, client)
				}
			case <-ctx.Done():
				return
			}
		}
	}(entries)

	for {
		err = resolver.Browse(ctx, "_workstation._tcp", "local.", entries)
		if err != nil {
			log.Fatalf("Failed to browse: %v", err)
		}
		printClients(clients)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 10):
		}
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
	ignoreNextChange = true

	for i := range clients {
		var client = &clients[i]
		serverAddr := fmt.Sprintf("%s:%d", client.IP, client.Port)

		var localIp, _ = getLocalIP()
		if string(client.IP) == localIp {
			continue
		}

		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			fmt.Println("Error connecting to server:", err)
			continue
		}

		defer conn.Close()

		go func() {
			ignoreNextChange = false
		}()
		time.Sleep(100 * time.Millisecond)

		_, err = conn.Write(data)
		if err != nil {
			println("Write to server failed:", err.Error())
			os.Exit(1)
		}
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

func listenForUdp(ctx context.Context) {
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
	defer conn.Close()

	buffer := make([]byte, BufferSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
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
}

func listenForTcp(ctx context.Context) {
	addr := fmt.Sprintf(":%d", PORT)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Printf("Listening on %s\n", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		go handleConnection(conn)

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	buffer := make([]byte, 1024)
	// Read the incoming connection into the buffer
	messageLength, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading from connection:", err)
		return
	}
	var message = buffer[:messageLength]
	fmt.Println("\nReceived message:", string(message))
	var currentData = clipboard.Read(clipboard.FmtText)
	if bytes.Equal(message, currentData) == false {
		clipboard.Write(clipboard.FmtText, []byte(fmt.Sprintf("Received from server: %s", string(message))))
	}
}

func watchClipboard(ctx context.Context) {
	ch := clipboard.Watch(ctx, clipboard.FmtText)

	for {
		select {
		case data := <-ch:
			sendData(data)
		case <-ctx.Done():
			return
		}
	}
}
