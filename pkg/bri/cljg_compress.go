// cljg_compress.go — the Go half of cljg.compress (ADR 0103 wave 1, spike
// s61): gzip / deflate (raw flate) / zlib compress+decompress over Go's
// stdlib compress/* packages — pure Go, zero deps, so CGO_ENABLED=0 + cljgo
// dist hold, and a non-OptIn namespace (nothing to isolate). zstd/brotli are
// DEFERRED (spike s61: pure-Go deps exist but cost ~4.9 MB of tables — they
// belong behind an opt-in link, not in every binary).
//
// Three shims carry all six public fns plus streaming: -compress and
// -decompress work on whole values (string or []byte in, []byte — or string,
// for text — out); -decompress-stream wraps a source of compressed data (a
// cljg.stream ReadableStream or a []byte) as a NEW ReadableStream that yields
// the decompressed bytes on demand, composing with the whole cljg.stream
// surface (read-all/lines/reduce). The ergonomic API is portable Clojure
// (core/cljg/compress.cljg). Interned as :private vars into cljg.compress.
//
// cljg.compress rides the same name-generic embedded-namespace registry as
// bri and the other cljg.* namespaces (the pkg/bri package name is a legacy
// of bri being the first tenant — ADR 0087 §1).
package bri

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// compressPublicName maps a codec + direction to the cljg.compress public fn
// name, so every error names the fn the USER called (error doctrine: name the
// thing), not an internal shim.
func compressPublicName(codec string, decompress bool) string {
	switch codec {
	case "gzip":
		if decompress {
			return "cljg.compress/gunzip"
		}
		return "cljg.compress/gzip"
	case "deflate":
		if decompress {
			return "cljg.compress/inflate"
		}
		return "cljg.compress/deflate"
	case "zlib":
		if decompress {
			return "cljg.compress/zlib-decompress"
		}
		return "cljg.compress/zlib-compress"
	}
	return "cljg.compress"
}

// compressInput coerces v (a string or any byte-array) to the bytes a codec
// consumes. It goes through toGoBytes so the SIGNED byte-arrays every cljg
// byte producer returns — cljg.io/read-bytes, cljg.security/base64-decode-bytes,
// cljg.stream/read-bytes — feed gunzip/inflate directly (ADR 0110: producers
// and consumers must compose; a value that answers `bytes?` true must be
// accepted wherever bytes are asked for).
func compressInput(name string, v any) []byte {
	return toGoBytes(name, v)
}

// compressLevel validates a flate-family compression level: -1 (default) or
// 0 (store) through 9 (best compression).
func compressLevel(name string, v any) int {
	level := asInt(v)
	if level < flate.DefaultCompression || level > flate.BestCompression {
		panic(fmt.Errorf("%s: :level must be -1 (default) or 0-9, got: %d", name, level))
	}
	return level
}

// newCompressWriter opens a streaming compressor for codec over w at level.
func newCompressWriter(name, codec string, w io.Writer, level int) io.WriteCloser {
	var cw io.WriteCloser
	var err error
	switch codec {
	case "gzip":
		cw, err = gzip.NewWriterLevel(w, level)
	case "deflate":
		cw, err = flate.NewWriter(w, level)
	case "zlib":
		cw, err = zlib.NewWriterLevel(w, level)
	default:
		err = fmt.Errorf("unknown codec %q (expected gzip, deflate or zlib)", codec)
	}
	if err != nil {
		panic(fmt.Errorf("%s: %w", name, err))
	}
	return cw
}

// newDecompressReader opens a streaming decompressor for codec over r. gzip
// and zlib read (and validate) their headers eagerly, so corrupt input fails
// here with a named error.
func newDecompressReader(name, codec string, r io.Reader) io.ReadCloser {
	var dr io.ReadCloser
	var err error
	switch codec {
	case "gzip":
		dr, err = gzip.NewReader(r)
	case "deflate":
		dr = flate.NewReader(r)
	case "zlib":
		dr, err = zlib.NewReader(r)
	default:
		err = fmt.Errorf("unknown codec %q (expected gzip, deflate or zlib)", codec)
	}
	if err != nil {
		panic(fmt.Errorf("%s: invalid %s data: %w", name, codec, err))
	}
	return dr
}

// compressCloser closes a decompressor and then the wrapped source, so
// closing a decompress-stream releases the subprocess pipe / HTTP body it
// draws from.
type compressCloser struct {
	decompressor io.Closer
	source       *ReadableStream
}

func (c *compressCloser) Close() error {
	err := c.decompressor.Close()
	if c.source != nil {
		c.source.closeRead()
	}
	return err
}

// installCompressShims interns cljg.compress's private Go codec primitives.
func installCompressShims(def func(name string, fn func(args ...any) any)) {
	// -compress (codec data level) -> compressed []byte. codec is "gzip",
	// "deflate" or "zlib"; data a string or []byte; level -1 or 0-9.
	def("-compress", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("-compress expects 3 args (codec data level), got %d", len(args)))
		}
		codec := asString(args[0])
		name := compressPublicName(codec, false)
		in := compressInput(name, args[1])
		level := compressLevel(name, args[2])
		var buf bytes.Buffer
		w := newCompressWriter(name, codec, &buf, level)
		if _, err := w.Write(in); err != nil {
			panic(fmt.Errorf("%s: %w", name, err))
		}
		if err := w.Close(); err != nil {
			panic(fmt.Errorf("%s: %w", name, err))
		}
		return toClojureBytes(buf.Bytes())
	})

	// -decompress (codec data as-string?) -> decompressed []byte, or a string
	// when as-string? is true. data is the compressed []byte (a string is
	// tolerated for byte-valued strings).
	def("-decompress", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("-decompress expects 3 args (codec data as-string), got %d", len(args)))
		}
		codec := asString(args[0])
		name := compressPublicName(codec, true)
		in := compressInput(name, args[1])
		r := newDecompressReader(name, codec, bytes.NewReader(in))
		out, err := io.ReadAll(r)
		if err == nil {
			err = r.Close()
		}
		if err != nil {
			panic(fmt.Errorf("%s: invalid %s data: %w", name, codec, err))
		}
		if args[2] == true {
			return string(out)
		}
		return toClojureBytes(out)
	})

	// -decompress-stream (codec source) -> a NEW ReadableStream yielding the
	// decompressed bytes. source is a cljg.stream ReadableStream of
	// compressed data, or a compressed []byte. Closing the returned stream
	// closes the source.
	def("-decompress-stream", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-decompress-stream expects 2 args (codec source), got %d", len(args)))
		}
		codec := asString(args[0])
		name := compressPublicName(codec, true) + "-stream"
		switch src := args[1].(type) {
		case *ReadableStream:
			dr := newDecompressReader(name, codec, src.br)
			return newReadableStream(dr, &compressCloser{decompressor: dr, source: src})
		case []byte:
			dr := newDecompressReader(name, codec, bytes.NewReader(src))
			return newReadableStream(dr, dr)
		case []int8:
			dr := newDecompressReader(name, codec, bytes.NewReader(toGoBytes(name, src)))
			return newReadableStream(dr, dr)
		default:
			panic(fmt.Errorf("%s expects a readable stream or byte-array, got: %s", name, lang.PrintString(args[1])))
		}
	})
}
