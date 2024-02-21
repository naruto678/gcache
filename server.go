package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
        "strconv"
        "bytes"
        "bufio"
        "log"
        "encoding/gob"
)

var clientParseError = errors.New("Unable to parse")

type container struct {
	mu   sync.Mutex
	data map[string]string
}

var bucket = container{
	data: map[string]string{},
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

type Response struct {
	Status int
	Data   string
}

func parseInput(line string) (*InputRequest, error) {

	// the thing looks like this
	//<command name> <key> <flags> <exptime> <byte count> [noreply]\r\n
	fields := strings.Fields(line)
	req := &InputRequest{}
	req.opts = map[string]bool{}
	if fields[0] == string(SET) {
		req.cmd = SET
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
		if err == nil {
			input, err := parseInput(line)
			if err != nil {
				fmt.Fprintf(conn, "error occurred %s", err.Error())
			}
			log.Printf("received input %v\n", input)
			var res Response
			if input.cmd == GET {
				res = handleGet(input)
			} else if input.cmd == SET {
				data := make([]byte, input.byteCount)
				_, err := reader.Read(data)
				if err != nil {
					fmt.Fprintf(conn, "error occurred %s", err.Error())
				}
				res = handleSet(input, string(data))
			}
			serializedRes, err := serializeResponse(res)
			if err != nil {
				fmt.Fprintf(conn, "error occured %s", err.Error())
				continue
			}
			n, err := conn.Write(serializedRes)
			if err != nil {
				fmt.Fprintf(conn, "error occured %s", err.Error())
				continue
			} else {
				log.Printf("Wrote %d bytes to the client connection\n", n)
			}

		} else {
			fmt.Fprintf(conn, "Error occured %s", err.Error())
		}

	}
}

func serializeResponse(res Response) ([]byte, error) {
	result := []byte{}
	buffer := bytes.NewBuffer(result)
	enc := gob.NewEncoder(buffer)
	err := enc.Encode(res)
	if err != nil {
		return []byte{}, err
	}
	return result, nil
}

func handleGet(request *InputRequest) Response {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	response := Response{}
	val, found := bucket.data[request.key]
	if !found {
		response.Status = 404
	} else {
		response.Status = 200
		response.Data = val
	}
	return response
}

func handleSet(request *InputRequest, data string) Response {

	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	res := Response{}
	bucket.data[request.key] = data
	res.Status = 200
	res.Data = data
	return res
}

type Server struct {
	host    string
	port    int
	sigChan chan os.Signal
}

func NewServer(host string, port int) Server {
	return Server{
		host:    host,
		port:    port,
		sigChan: make(chan os.Signal),
	}
}

func (s Server) Run() {

	signal.Notify(s.sigChan, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT)

	conn, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.host, s.port))
	defer conn.Close()
	if err != nil {
		panic(err)
	}

	go func() {

		for {
			clientConn, err := conn.Accept()
			if err != nil {
				fmt.Println(err)
			}
			go func(conn net.Conn) {
				defer clientConn.Close()
				handle(conn)
			}(clientConn)
		}

	}()
	sig := <-s.sigChan
	fmt.Printf("Received %v signal. doing graceful shutdown\n", sig)
}

func (s Server) Close() {
	s.sigChan <- syscall.SIGINT
}
