package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var clientParseError = errors.New("Unable to parse")

type container struct {
	mu       sync.Mutex
	data     map[string]string
	flags    map[string]int
	exptimes map[string]time.Time
}

var bucket = container{
	data:     map[string]string{},
	flags:    map[string]int{},
	exptimes: map[string]time.Time{},
}

type Command string

const NOREPLY = "noreply"

var (
	GET Command = "get"
	SET Command = "set"
)

type InputRequest struct {
	cmd         Command
	key         string
	flag        int
	exptime     int
	byteCount   int
	opts        map[string]bool
	requestTime time.Time
}

type Response struct {
	Status    int
	Data      string
	flag      int
	byteCount int
	exptime   time.Time
}

func parseInput(line string) (*InputRequest, error) {

	// the thing looks like this
	//<command name> <key> <flags> <exptime> <byte count> [noreply]\r\n
	fields := strings.Fields(line)
	req := &InputRequest{
		requestTime: time.Now(),
	}
	req.opts = map[string]bool{}

	log.Printf("Got line %s before parsing\n", fields)

	if fields[0] == string(SET) {
		if len(fields) < 5 {
			return nil, clientParseError
		}
		req.cmd = SET
		flagField, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, err
		}
		req.flag = int(flagField)
		req.key = fields[1]
		expField, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, err
		}
		req.exptime =int(expField)
		byteField, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, clientParseError
		}
		req.byteCount = int(byteField)
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
				break
			}
			//log.Printf("received input %v\n", input)
			var res Response
			if input.cmd == GET {
				res = handleGet(input)
				log.Println("GET", res.Status, res.Data)
				if res.Status != 200 {
					write(conn, "END\n", input)
				} else {
					write(conn, fmt.Sprintf("VALUE %s %d %d\n", input.key, res.flag, res.byteCount), input)
					write(conn, fmt.Sprintf("%s\n", res.Data), input)
					write(conn, "END\n", input)
				}
			} else if input.cmd == SET {
				data, err := reader.ReadString('\n')
				if err != nil {
					fmt.Fprintf(conn, "error occured %s", err.Error())
				}
				res = handleSet(input, string([]byte(data)[:input.byteCount]))
				if res.Status == 200 {
					if !input.opts[NOREPLY] {
						write(conn, "STORED\n", input)
					}
				}
				log.Println("SET", res.Status, res.Data)
			}

		} else {
			fmt.Fprintf(conn, "Error occured %s", err.Error())
		}

	}
}

func write(conn net.Conn, data string, input *InputRequest) {
	if input.opts[NOREPLY] {
		return
	}
	_, err := conn.Write([]byte(data))
	if err != nil {
		log.Println(err)
		return
	}
	//log.Printf("Wrote %d bytes to the client\n", n)
}

func serializeResponse(res Response) ([]byte, error) {
	buffer := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(buffer)
	err := enc.Encode(res)
	if err != nil {
		log.Println(err.Error())
	}
	return buffer.Bytes(), nil
}

func handleGet(request *InputRequest) Response {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	response := Response{}
	val, found := bucket.data[request.key]
	if !found {
		response.Status = 404
	} else {
		flag := bucket.flags[request.key]
		exptime := bucket.exptimes[request.key]

		if request.requestTime.After(exptime) {
			log.Printf("Deleting the expired value from the bucket")
			delete(bucket.data,request.key)
			delete(bucket.exptimes, request.key)
			delete(bucket.flags, request.key)
			response.Status = 300
		} else {
			response.flag = flag
			response.exptime = exptime
			response.Status = 200
			response.Data = val
			response.byteCount = len(val)
		}
	}
	return response
}

func handleSet(request *InputRequest, data string) Response {
	//log.Printf("received set request [%v] with data [%s] ", request, data)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	res := Response{}
	bucket.data[request.key] = data
	bucket.flags[request.key] = request.flag
	bucket.exptimes[request.key] = request.requestTime.Add(time.Duration(request.exptime) * time.Second)
	res.Status = 200
	res.Data = data
	//log.Printf("set key = %s for value %s\n", request.key, data)
	//log.Printf("bucket %v", bucket.data)
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
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Printf("Listening on incoming connections on %s:%d\n", s.host, s.port)

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
