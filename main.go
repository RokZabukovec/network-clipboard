package main

func main() {
	service := NewService("NetClip", 8080)

	go service.Register()

	server := NewServer(service.Port)

	clients := NewClientPool()

	go clients.Browse()

	server.listen(ConnHandler)
}
