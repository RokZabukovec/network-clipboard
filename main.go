package main

func main() {
	server := NewServer("NetClip", 8080)

	server.InitAndListen(ConnHandler)
}
