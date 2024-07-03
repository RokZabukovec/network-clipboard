package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gen2brain/beeep"
	"github.com/google/uuid"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
	"golang.design/x/clipboard"
)

var uniqueId = uuid.New().String()

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

func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func registerService(ctx context.Context) {
	ip := getOutboundIP()

	txtRecords := []string{
		"uuid=" + uniqueId,
		"ip=" + ip.String(),
	}

	server, err := zeroconf.Register(ServiceName, "_workstation._tcp", "local.", PORT, txtRecords, nil)
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
				ip := net.ParseIP(getServiceIp(entry))

				client := Client{
					IP:   ip,
					Port: entry.Port,
					Name: entry.HostName,
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
	//buffer := make([]byte, 1024)
	// Read the incoming connection into the buffer
	data, _ := io.ReadAll(conn)

	if len(data) == 0 {
		return
	}
	fmt.Println("\nReceived message:", string(data))
	var currentData = clipboard.Read(clipboard.FmtText)
	beeep.Notify("Message received", string(data), "")

	if bytes.Equal(data, currentData) == false {
		clipboard.Write(clipboard.FmtText, data)
		beeep.Notify("Network clipboard", string(data), "")
	}
	response := []byte("Message received")
	_, err := conn.Write(response)
	if err != nil {
		fmt.Println("Error writing to connection:", err)
		return
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

func isCurrentInstance(entry *zeroconf.ServiceEntry) bool {
	var serviceTxtPrefix = "uuid="

	for _, txt := range entry.Text {
		var instanceId, found = strings.CutPrefix(txt, serviceTxtPrefix)

		if !found {
			continue
		}

		return instanceId == uniqueId
	}

	return false
}

func getServiceIp(entry *zeroconf.ServiceEntry) string {
	var ipTxtPrefix = "ip="

	for _, txt := range entry.Text {
		var ip, found = strings.CutPrefix(txt, ipTxtPrefix)

		if !found {
			continue
		}

		return ip
	}

	return ""
}
