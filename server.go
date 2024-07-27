package main

import (
	"bytes"
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/gen2brain/beeep"
	"golang.design/x/clipboard"
	"net"
)

type ConnectionHandler func(net.Conn)

type Server struct {
	Port int
}

func NewServer(port int) *Server {
	return &Server{port}
}

func (s *Server) listen(handler ConnectionHandler) {
	log.Info("Server listening", "port", s.Port)
	addr := fmt.Sprintf(":%d", s.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		go handler(conn)
	}
}

func ConnHandler(conn net.Conn) {
	defer conn.Close()
	buffer := make([]byte, 1024)
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
		beeep.Notify("Network clipboard", "Clipboard updated with new data", "")
	}
}
