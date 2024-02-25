package main

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestServer(t *testing.T) {
	server := NewServer("", 8080)
	go func() { server.Run() }()
	time.Sleep(5 * time.Second)
	conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	reader := bufio.NewReader(conn)
	conn.Write([]byte("GET test\n"))
	time.Sleep(2 * time.Second)
	line, err := reader.ReadString('\n')
	fmt.Println(line)
	server.Close()
}
