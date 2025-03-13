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
	"io"
	"net"
)

type ConnectionHandler func(net.Conn)

type Server struct {
	UUID         uuid.UUID
	Addr         string
	Name         string
	Port         int
	CopiedData   chan []byte
	ReceivedData chan []byte
	Peers        []*Client
	Ip           net.IP
	Ln           net.Listener
	Quit         chan struct{}
}

func NewServer(name string, port int) *Server {
	ip := GetIp()
	addr := fmt.Sprintf(":%d", port)
	log.Info("Server is listening", "addr", addr)

	return &Server{
		UUID:       uuid.New(),
		Addr:       addr,
		Name:       name,
		Port:       port,
		CopiedData: make(chan []byte),
		Peers:      make([]*Client, 0),
		Ip:         ip,
		Quit:       make(chan struct{}),
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

func (s *Server) Start() {
	s.Init()
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		panic("Error starting server")
	}
	defer listener.Close()
	s.Ln = listener
	fmt.Printf("Server is listening on port :%d\n", s.Port)

	go s.acceptLoop()

	<-s.Quit
}

func (s *Server) acceptLoop() {
	for {
		connection, err := s.Ln.Accept()

		if err != nil {
			fmt.Println("Error accepting connection")
			continue
		}

		fmt.Println("Connection accepted from: ", connection.RemoteAddr())

		go s.readLoop(connection)
	}
}

func (s *Server) readLoop(conn net.Conn) {
	defer func() {
		fmt.Println("Connection closed by remote:", conn.RemoteAddr())
		conn.Close()
	}()

	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("Client disconnected:", conn.RemoteAddr())
				break
			}
			fmt.Println("Error reading from connection:", err)
			break
		}

		_, _ = conn.Write([]byte("You typed: " + line))

		clipboardHandle([]byte(line))
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
		s.CopiedData <- data
	}
}

func (s *Server) BroadcastMessages() {
	for message := range s.CopiedData {
		fmt.Print(string(message))
	}
}
