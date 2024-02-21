package main

import (
	"io"
	"net"
	"testing"
        "fmt"
)

func testSet(t *testing.T) {
	go func() { main() }() // start the server 
        conn, err := net.Dial("tcp", "8080")
        defer conn.Close()

        if err!=nil{
                panic(err)
        }
        conn.Write([]byte("GET 0"))
        result, err := io.ReadAll(conn)
        if err!=nil{
                t.Fatal(err)
        }
        fmt.Println(result)
}
