// go run . -port 1667 — then connect Calva ("Connect to a running
// REPL", generic nREPL) or CIDER (cider-connect-clj) to localhost:1667.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
)

func main() {
	port := flag.Int("port", 1667, "TCP port to listen on")
	flag.Parse()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("nREPL server started on port %d on host 127.0.0.1 - nrepl://127.0.0.1:%d\n",
		*port, *port)
	newServer().serve(ln)
}
