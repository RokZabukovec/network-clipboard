package main

import (
	"context"
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
	"time"
)

type Client struct {
	ip string
}

type ClientPool struct {
	clients []*Client
}

func NewClientPool() *ClientPool {
	return &ClientPool{clients: make([]*Client, 0)}
}

func (cp *ClientPool) Browse() {
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

				if cp.containsClient(&client) == false {
					cp.AddClient(&client)
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

func (cp *ClientPool) AddClient(client *Client) {
	if !cp.containsClient(client) {
		cp.clients = append(cp.clients, client)
		fmt.Println("Client added:", client.ip)
	} else {
		fmt.Println("Client already exists:", client.ip)
	}
}

func (cp *ClientPool) containsClient(newClient *Client) bool {
	for _, client := range cp.clients {
		if client.ip == newClient.ip {
			return true
		}
	}
	return false
}
