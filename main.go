package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var clientParseError = errors.New("Unable to parse")

type container struct {
	mu   sync.Mutex
	data map[string]string
}

var bucket = container{
        data : map[string]string{ }, 
}
type Command string

const NOREPLY = "noreply"

var (
	GET Command = "GET"
	SET Command = "SET"
)

type InputRequest struct {
	cmd       Command
	key       string
	flag      uint16
	exptime   uint16
	byteCount uint64
	opts      map[string]bool
}

func parseInput(line string) (*InputRequest, error) {

	// the thing looks like this
	//<command name> <key> <flags> <exptime> <byte count> [noreply]\r\n
	fields := strings.Fields(line)
	req := &InputRequest{}
	req.opts = map[string]bool{}
	if fields[0] == string(SET) {
		req.cmd = GET
		flagField, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return nil, err
		}
		req.flag = uint16(flagField)
		req.key = fields[1]
		expField, err := strconv.ParseUint(fields[3], 10, 64)
		if err != nil {
			return nil, err
		}
		req.exptime = uint16(expField)
		byteField, err := strconv.ParseUint(fields[4], 10, 64)
		if err != nil {
			req.byteCount = byteField
		}
		if len(fields) > 5 && fields[5] == NOREPLY {
			req.opts[NOREPLY] = true
		}
		return req, nil
	} else if fields[0] == string(GET) {
		if len(fields) < 1 {
			return nil, clientParseError
		}
		key := fields[1]
		req.cmd = GET
		req.key = key
		return req, nil
	}

	return nil, clientParseError
}

func handle(conn net.Conn) {
	fmt.Fprintf(conn, "Hello client. this is the memache server\n")
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			input, err := parseInput(line)
			if err != nil {
				fmt.Fprintf(conn, "error occurred %s", err.Error())
			}
			if input.cmd == GET {
				handleGet(input)
			} else if input.cmd == SET {
                                
				handleSet(input)
			}

		}

	}
}

func handleGet(request *InputRequest) {
        bucket.mu.Lock()
        defer bucket.mu.Unlock()
        byteCount := request.byteCount

}

func handleSet(request *InputRequest){}

func main() {
	var (
		hostname string
		port     int
	)

	flag.StringVar(&hostname, "host", "127.0.0.1", "set the hostname")
	flag.IntVar(&port, "port", 8080, "set the port")
	flag.Parse()

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	conn, err := net.Listen("tcp", fmt.Sprintf("%s:%d", hostname, port))
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stdout, "Listening for connections on [%s:%d]", hostname, port)

	go func() {
		for {
			clientConn, err := conn.Accept()
			if err != nil {
				fmt.Println(err)
			}
			go func(conn net.Conn) {
				defer conn.Close()
				handle(conn)
			}(clientConn)
		}
	}()
	recSig := <-sigChan
	fmt.Fprintf(os.Stdout, "Received %s signal. Doing graceful exit of the server.\n", recSig)
}
