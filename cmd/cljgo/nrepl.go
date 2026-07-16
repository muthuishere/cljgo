// cljgo nrepl — the nREPL server frontend (ADR 0031). Default port 0 =
// an ephemeral port, printed in nrepl.cmdline's banner format, plus a
// .nrepl-port file (just the digits) in the cwd so editors auto-discover
// the server — the convention real nREPL documents
// (nrepl.org/nrepl/usage/server.html: nrepl.cmdline "writes the server
// port to a file so that editors and other tools can automatically find
// and connect", removed on shutdown).
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"

	"github.com/muthuishere/cljgo/pkg/nrepl"
)

const portFileName = ".nrepl-port"

func runNREPL(args []string) int {
	fs := flag.NewFlagSet("nrepl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	port := fs.Int("port", 0, "TCP port to listen on (0 = an ephemeral port)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo nrepl [--port N]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer ln.Close()
	actual := ln.Addr().(*net.TCPAddr).Port

	// The port file: digits only, cwd, best-effort (a read-only cwd must
	// not stop the server), removed on shutdown like nrepl.cmdline's.
	if err := os.WriteFile(portFileName, []byte(strconv.Itoa(actual)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", portFileName, err)
	} else {
		defer os.Remove(portFileName)
	}

	// Ctrl-C: close the listener so Serve returns and the deferred
	// port-file removal runs (a bare os.Exit would leak the file).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		ln.Close()
	}()

	fmt.Printf("nREPL server started on port %d on host 127.0.0.1 - nrepl://127.0.0.1:%d\n",
		actual, actual)
	nrepl.NewServer().Serve(ln)
	return 0
}
