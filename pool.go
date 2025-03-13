package main

import (
	"context"
	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
	"time"
)

type Client struct {
	ip string
}

func (s *Server) Browse() {
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
					ip: entry.AddrIPv4[0].String(),
				}

				if s.containsClient(&client) == false {
					s.AddPeer(&client)
				}
			}
		}
	}(entries)

	for {
		err = resolver.Browse(context.Background(), "_netclip._tcp", "local.", entries)
		if err != nil {
			log.Fatalf("Failed to browse: %v", err)
		}

		select {
		case <-time.After(time.Second * 10):
		}
	}
}
