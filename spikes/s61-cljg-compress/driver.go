package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"os"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// payload — repetitive enough to actually compress, but with some variety.
var payload = func() []byte {
	var b bytes.Buffer
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "cljg.compress round-trip line %d — the quick brown fox jumps over the lazy dog\n", i)
	}
	return b.Bytes()
}()

// roundTrip drives a streaming compressor and decompressor as io.Writer/io.Reader
// wrappers, exactly the way cljg.stream would compose them. It returns the
// compressed size and whether the decompressed bytes equal the original.
func roundTrip(
	name string,
	newW func(io.Writer) (io.WriteCloser, error),
	newR func(io.Reader) (io.ReadCloser, error),
) {
	// --- compress (streaming write) ---
	var comp bytes.Buffer
	w, err := newW(&comp)
	if err != nil {
		fail(name, "new writer", err)
	}
	if _, err := w.Write(payload); err != nil {
		fail(name, "write", err)
	}
	if err := w.Close(); err != nil {
		fail(name, "close writer", err)
	}

	// --- decompress (streaming read) ---
	r, err := newR(bytes.NewReader(comp.Bytes()))
	if err != nil {
		fail(name, "new reader", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		fail(name, "read", err)
	}
	if err := r.Close(); err != nil {
		fail(name, "close reader", err)
	}

	equal := bytes.Equal(got, payload)
	ratio := float64(comp.Len()) / float64(len(payload)) * 100
	status := "OK"
	if !equal {
		status = "MISMATCH"
	}
	fmt.Printf("%-9s in=%d bytes  out=%d bytes  ratio=%.1f%%  roundtrip=%s\n",
		name, len(payload), comp.Len(), ratio, status)
	if !equal {
		os.Exit(1)
	}
}

func fail(name, stage string, err error) {
	fmt.Printf("%-9s FAILED at %s: %v\n", name, stage, err)
	os.Exit(1)
}

// nopWriteCloser wraps writers whose Close doesn't fit (flate.Writer needs no
// re-wrap actually; kept simple below).

func main() {
	fmt.Printf("cljg.compress spike — payload %d bytes\n\n", len(payload))

	// gzip — stdlib
	roundTrip("gzip",
		func(w io.Writer) (io.WriteCloser, error) { return gzip.NewWriter(w), nil },
		func(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) },
	)

	// flate/deflate — stdlib
	roundTrip("flate",
		func(w io.Writer) (io.WriteCloser, error) { return flate.NewWriter(w, flate.DefaultCompression) },
		func(r io.Reader) (io.ReadCloser, error) { return flate.NewReader(r), nil },
	)

	// zlib — stdlib
	roundTrip("zlib",
		func(w io.Writer) (io.WriteCloser, error) { return zlib.NewWriter(w), nil },
		func(r io.Reader) (io.ReadCloser, error) { return zlib.NewReader(r) },
	)

	// zstd — github.com/klauspost/compress/zstd (pure Go)
	roundTrip("zstd",
		func(w io.Writer) (io.WriteCloser, error) {
			zw, err := zstd.NewWriter(w)
			return zw, err
		},
		func(r io.Reader) (io.ReadCloser, error) {
			zr, err := zstd.NewReader(r)
			if err != nil {
				return nil, err
			}
			return zr.IOReadCloser(), nil
		},
	)

	// brotli — github.com/andybalholm/brotli (pure Go)
	roundTrip("brotli",
		func(w io.Writer) (io.WriteCloser, error) { return brotli.NewWriter(w), nil },
		func(r io.Reader) (io.ReadCloser, error) {
			return io.NopCloser(brotli.NewReader(r)), nil
		},
	)

	fmt.Println("\nAll round-trips OK.")
}
