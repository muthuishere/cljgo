// Spike s63-cljg-ws: prove cljg.ws (raw websocket duplex primitive) in pure Go, CGO=0.
//
// Round-trip: start an HTTP server that upgrades to a websocket and echoes;
// dial it as a client; send a TEXT frame and a BINARY frame; assert the echoed
// payloads match. This exercises the real coder/websocket API (Accept / Dial /
// Read / Write), which is the shape cljg.ws would wrap as a duplex stream.
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"
)

// echoServer upgrades the request to a websocket and echoes every frame back,
// preserving the frame's message type (text vs binary). This is the server half
// of a duplex read-frame/write-frame stream.
func echoHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("server accept: %v", err)
		return
	}
	defer c.CloseNow()
	ctx := r.Context()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			// normal close when client goes away
			return
		}
		if err := c.Write(ctx, typ, data); err != nil {
			return
		}
	}
}

func main() {
	// Bind an ephemeral localhost port for a real network round-trip.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	srv := &http.Server{Handler: http.HandlerFunc(echoHandler)}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Printf("echo ws server listening on %s\n", addr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- DIAL (client side of the duplex stream) ---
	url := "ws://" + addr
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()
	fmt.Printf("client dialled %s\n", url)

	ok := true

	// --- TEXT frame round-trip ---
	textOut := "hello cljg.ws — unicode ✓"
	if err := conn.Write(ctx, websocket.MessageText, []byte(textOut)); err != nil {
		log.Fatalf("write text: %v", err)
	}
	typ, textIn, err := conn.Read(ctx)
	if err != nil {
		log.Fatalf("read text: %v", err)
	}
	fmt.Printf("TEXT   sent=%q recv=%q type=%v\n", textOut, string(textIn), typ)
	if typ != websocket.MessageText || string(textIn) != textOut {
		fmt.Println("TEXT   MISMATCH")
		ok = false
	} else {
		fmt.Println("TEXT   OK (echoed payload + type match)")
	}

	// --- BINARY frame round-trip ---
	binOut := []byte{0x00, 0x01, 0x02, 0xfe, 0xff, 0x7f, 0x80}
	if err := conn.Write(ctx, websocket.MessageBinary, binOut); err != nil {
		log.Fatalf("write binary: %v", err)
	}
	typ, binIn, err := conn.Read(ctx)
	if err != nil {
		log.Fatalf("read binary: %v", err)
	}
	fmt.Printf("BINARY sent=% x recv=% x type=%v\n", binOut, binIn, typ)
	if typ != websocket.MessageBinary || !bytes.Equal(binIn, binOut) {
		fmt.Println("BINARY MISMATCH")
		ok = false
	} else {
		fmt.Println("BINARY OK (echoed bytes + type match)")
	}

	// Clean protocol-level close of the duplex stream.
	conn.Close(websocket.StatusNormalClosure, "done")

	if ok {
		fmt.Println("RESULT: PASS - duplex ws round-trip (text + binary) succeeded")
	} else {
		fmt.Println("RESULT: FAIL")
		os.Exit(1)
	}
}
