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
	"strconv"
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
	interfaces, _ := net.Interfaces()
	fmt.Println(interfaces)
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
	txtRecords := []string{
		"uuid=" + uniqueId,
	}

	wifiInterface, err := GetWifiInterface()

	server, err := zeroconf.Register(ServiceName, "_workstation._tcp", "local.", PORT, txtRecords, []net.Interface{*wifiInterface})
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
				var isMe = isCurrentInstance(entry)
				if isMe {
					continue
				}
				for index := range entry.AddrIPv4 {
					if !isReachable(entry.AddrIPv4[index], PORT) {
						continue
					}

					client := Client{
						IP:   entry.AddrIPv4[index],
						Port: entry.Port,
						Name: entry.HostName,
					}

					if !clientExists(clients, client) {
						*clients = append(*clients, client)
					}
					break
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

func isReachable(ip net.IP, port int) bool {
	if ip == nil {
		return false
	}
	address := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	_, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return false
	}

	return true
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
		if client.IP.String() == localIp {
			fmt.Println("\nSkip yourself")
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

// GetWifiInterface returns the network interface named "WiFi"
func GetWifiInterface() (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		if iface.Name == "WiFi" {
			return &iface, nil
		}
	}
	return nil, fmt.Errorf("Wi-Fi interface not found")
}
