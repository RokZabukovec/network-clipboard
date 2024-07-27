package main

import (
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/grandcat/zeroconf"
	"net"
)

type Service struct {
	Name string
	Port int
}

func NewService(name string, port int) *Service {
	return &Service{Name: name, Port: port}
}

func (s *Service) getIp() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")

	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func (s *Service) Register() {
	ip := s.getIp()
	uniqueId := uuid.New().String()

	txtRecords := []string{
		"uuid=" + uniqueId,
		"ip=" + ip.String(),
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
