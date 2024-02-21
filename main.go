package main


import "flag"

func main() {
	var (
		hostname string
		port     int
	)

	flag.StringVar(&hostname, "host", "127.0.0.1", "set the hostname")
	flag.IntVar(&port, "port", 8080, "set the port")
	flag.Parse()
        server := NewServer(hostname, port)
        server.Run()
}
