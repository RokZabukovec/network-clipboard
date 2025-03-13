package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/gen2brain/beeep"
	"github.com/google/uuid"
	"github.com/grandcat/zeroconf"
	"golang.design/x/clipboard"
	"net"
	"os"
)

type ConnectionHandler func(net.Conn)

type Server struct {
	UUID     uuid.UUID
	Addr     string
	Name     string
	Port     int
	Messages chan []byte
	Peers    []*Client
	Ip       net.IP
}

func NewServer(name string, port int) *Server {
	ip := GetIp()
	addr := fmt.Sprintf(":%d", port)
	log.Info("Server is listening", "addr", addr)

	return &Server{
		UUID:     uuid.New(),
		Addr:     addr,
		Name:     name,
		Port:     port,
		Messages: make(chan []byte),
		Peers:    make([]*Client, 0),
		Ip:       ip,
	}
}

func (s *Server) Init() {
	err := clipboard.Init()
	if err != nil {
		panic(err)
	}

	go s.Register()
	go s.Browse()
	go s.WatchClipboard()
	go s.BroadcastMessages()
}

func (s *Server) InitAndListen(handler ConnectionHandler) {
	s.Init()
	// Start a TCP server listening on port 8080
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("Server is listening on port 8080...")

	for {
		// Accept an incoming connection
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		fmt.Println("New client connected:", conn.RemoteAddr())

		// Handle the client in a separate goroutine
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Create a buffered reader to read client messages
	reader := bufio.NewReader(conn)

	for {
		// Read the message from the client
		message, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Client disconnected:", conn.RemoteAddr())
			return
		}

		fmt.Printf("Received: %s", message)

		// Send a response back to the client
		_, err = conn.Write([]byte("Message received: " + message))
		if err != nil {
			fmt.Println("Error sending response:", err)
			return
		}
	}
}

func (s *Server) Register() {
	txtRecords := []string{
		"uuid=" + s.UUID.String(),
		"ip=" + s.Ip.String(),
	}

	server, err := zeroconf.Register(s.Name, "_netclip._tcp", "local.", s.Port, txtRecords, nil)

	if err != nil {
		panic(err)
	}

	defer server.Shutdown()
	defer log.Info("Service is shutting down")
	log.Info("Service started broadcasting")

	select {}
}

func ConnHandler(conn net.Conn) {
	defer conn.Close()
	buffer := make([]byte, 2048)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				log.Error("Client disconnected")
			} else {
				log.Error("Error reading data", "error", err)
			}
			return
		}

		message := string(buffer[:n])
		clipboardHandle(buffer[:n])
		log.Print("Message received: ", "message", message)

		_, err = conn.Write([]byte("Thank you"))

		if err != nil {
			log.Error("Error sending data:", err)

			return
		}
	}
}

func clipboardHandle(data []byte) {
	var currentData = clipboard.Read(clipboard.FmtText)
	if !bytes.Equal(data, currentData) {
		clipboard.Write(clipboard.FmtText, data)
		err := beeep.Notify("Network clipboard", "Clipboard updated with new data", "")
		if err != nil {
			return
		}
	}
}

func GetIp() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")

	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func (s *Server) AddPeer(peer *Client) {
	if !s.containsClient(peer) {
		s.Peers = append(s.Peers, peer)
		fmt.Println("Client added:", peer.ip)
	} else {
		fmt.Println("Client already exists:", peer.ip)
	}
}

func (s *Server) containsClient(newClient *Client) bool {
	for _, client := range s.Peers {
		if client.ip == newClient.ip {
			return true
		}
	}
	return false
}

func (s *Server) WatchClipboard() {
	ch := clipboard.Watch(context.Background(), clipboard.FmtText)
	for data := range ch {
		s.Messages <- data
	}
}

func (s *Server) BroadcastMessages() {
	for message := range s.Messages {
		fmt.Print(string(message))
	}
}
