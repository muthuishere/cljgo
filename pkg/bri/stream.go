// stream.go — the Go half of cljg.stream (ADR 0101, spike s56): ONE reducible
// stream abstraction shared by cljg.process (subprocess pipes) and cljg.net.http
// (:as :stream response bodies). A ReadableStream wraps a Go io.Reader and is
// SEQABLE — (reduce f init readable) / (into [] readable) / (doseq …) walk it a
// chunk at a time in constant memory, and `reduced`/`take` short-circuit the
// underlying read (the same bet pkg/lang's LongRange/LazySeq make). A
// WritableStream wraps an io.Writer with buffered write + flush + close.
//
// Pure Go over stdlib bufio/io — CGO_ENABLED=0 + cljgo dist hold, and
// cljg.stream stays a non-OptIn namespace (no dependency to isolate). The
// ergonomic surface (chunks/lines/write/close) is portable Clojure
// (core/cljg/stream.cljg). Interned as :private vars into cljg.stream.
//
// cljg.stream rides the same name-generic embedded-namespace registry as bri
// and the other cljg.* namespaces (the pkg/bri package name is a legacy of bri
// being the first tenant — ADR 0087 §1).
package bri

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// defaultChunkSize is the byte-chunk size a ReadableStream hands out when the
// caller does not specify one (64 KiB — one bufio fill).
const defaultChunkSize = 64 * 1024

// ReadableStream is a single-pass, seqable view over a Go io.Reader: a
// subprocess's stdout/stderr, an HTTP response body, or any other stream. It
// is NOT re-readable — each element consumed advances the underlying reader;
// seqing it twice continues where the first left off (single-consumer, like the
// JVM's line-seq over a Reader). Backpressure is inherent: a chunk is produced
// only when the consumer asks (lazy-seq / read-line / read-bytes), so a slow
// consumer naturally throttles the producer through the OS pipe / TCP window.
type ReadableStream struct {
	br     *bufio.Reader
	closer io.Closer
	closed bool
}

// WritableStream is a buffered sink over a Go io.Writer: a subprocess's stdin,
// or any other writer. write flushes each call so the peer sees the bytes
// immediately (needed for a line-at-a-time protocol); close flushes then closes
// the underlying pipe, sending EOF to the reader on the other end.
type WritableStream struct {
	bw     *bufio.Writer
	closer io.Closer
	closed bool
}

// newReadableStream wraps r (and its closer, which may equal r) as a
// ReadableStream. closer may be nil for a reader with nothing to release.
func newReadableStream(r io.Reader, closer io.Closer) *ReadableStream {
	return &ReadableStream{br: bufio.NewReaderSize(r, defaultChunkSize), closer: closer}
}

// newWritableStream wraps w (and its closer) as a WritableStream.
func newWritableStream(w io.Writer, closer io.Closer) *WritableStream {
	return &WritableStream{bw: bufio.NewWriter(w), closer: closer}
}

// readBytes reads up to n bytes (n<=0 → defaultChunkSize), returning them as a
// []byte, or nil at end-of-stream. Fewer than n bytes is normal (whatever is
// available now) — the seq keeps pulling until EOF returns nil.
func (rs *ReadableStream) readBytes(n int) []byte {
	if n <= 0 {
		n = defaultChunkSize
	}
	if rs.closed {
		return nil
	}
	buf := make([]byte, n)
	for {
		m, err := rs.br.Read(buf)
		if m > 0 {
			return buf[:m]
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			panic(fmt.Errorf("cljg.stream: read: %w", err))
		}
		// m == 0, err == nil: rare, retry.
	}
}

// readLine reads one line (up to and including '\n'), returning it WITHOUT the
// trailing '\n' (and a trailing '\r' if present), or nil at end-of-stream. A
// final unterminated line is returned as-is.
func (rs *ReadableStream) readLine() any {
	if rs.closed {
		return nil
	}
	line, err := rs.br.ReadString('\n')
	if err == io.EOF {
		if line == "" {
			return nil
		}
		return trimEOL(line)
	}
	if err != nil {
		panic(fmt.Errorf("cljg.stream: read-line: %w", err))
	}
	return trimEOL(line)
}

// trimEOL strips a trailing "\n" or "\r\n" from a line.
func trimEOL(s string) string {
	if n := len(s); n > 0 && s[n-1] == '\n' {
		if n > 1 && s[n-2] == '\r' {
			return s[:n-2]
		}
		return s[:n-1]
	}
	return s
}

// readAll drains the rest of the stream into one string.
func (rs *ReadableStream) readAll() string {
	if rs.closed {
		return ""
	}
	b, err := io.ReadAll(rs.br)
	if err != nil {
		panic(fmt.Errorf("cljg.stream: read-all: %w", err))
	}
	return string(b)
}

// closeRead releases the underlying reader (idempotent). Consuming past this
// point yields nil / "".
func (rs *ReadableStream) closeRead() {
	if rs.closed {
		return
	}
	rs.closed = true
	if rs.closer != nil {
		_ = rs.closer.Close()
	}
}

// Seq makes a ReadableStream a first-class reducible: (reduce f init readable),
// (into [] readable), (doseq [c readable] …) all walk it as a lazy seq of
// []byte chunks (defaultChunkSize each), pulled on demand — constant memory,
// `reduced`/`take` short-circuit the read. This is THE decision (spike s56):
// one Seqable, and the whole reduce/transduce/into library applies to a live
// stream with no new protocol.
func (rs *ReadableStream) Seq() lang.ISeq {
	return rs.chunkSeq(defaultChunkSize)
}

// chunkSeq is the lazy backbone: read one chunk, cons it onto the lazy tail, or
// terminate at EOF.
func (rs *ReadableStream) chunkSeq(n int) lang.ISeq {
	return lang.NewLazySeq(func() any {
		chunk := rs.readBytes(n)
		if chunk == nil {
			return nil
		}
		return lang.NewCons(chunk, rs.chunkSeq(n))
	})
}

// lineSeq is the line-oriented lazy backbone (cljg.stream/lines).
func (rs *ReadableStream) lineSeq() lang.ISeq {
	return lang.NewLazySeq(func() any {
		line := rs.readLine()
		if lang.IsNil(line) {
			return nil
		}
		return lang.NewCons(line, rs.lineSeq())
	})
}

// write appends v (a string or []byte) to the buffer and flushes so the peer
// sees it now. Writing to a closed stream is an error.
func (ws *WritableStream) write(v any) {
	if ws.closed {
		panic(fmt.Errorf("cljg.stream: write to a closed stream"))
	}
	var err error
	switch b := v.(type) {
	case string:
		_, err = ws.bw.WriteString(b)
	case []byte:
		_, err = ws.bw.Write(b)
	case []int8:
		// what clojure.core/byte-array builds (ADR 0110 ask 1) — an honest
		// byte-array on this host, so it writes like []byte.
		_, err = ws.bw.Write(toGoBytes("cljg.stream/write", b))
	default:
		panic(fmt.Errorf("cljg.stream: write expects a string or byte-array, got: %s", lang.PrintString(v)))
	}
	if err == nil {
		err = ws.bw.Flush()
	}
	if err != nil {
		panic(fmt.Errorf("cljg.stream: write: %w", err))
	}
}

// closeWrite flushes then closes the underlying writer (idempotent), sending
// EOF to the reader on the other end of a pipe.
func (ws *WritableStream) closeWrite() {
	if ws.closed {
		return
	}
	ws.closed = true
	_ = ws.bw.Flush()
	if ws.closer != nil {
		_ = ws.closer.Close()
	}
}

// installStreamShims interns cljg.stream's private Go primitives. Both the
// readable and writable handles are opaque to Clojure; these shims are the only
// way to touch them, and the ergonomic chunks/lines/write/close live in
// core/cljg/stream.cljg.
func installStreamShims(def func(name string, fn func(args ...any) any)) {
	// -stream-read-bytes (readable n) -> []byte chunk, or nil at EOF.
	def("-stream-read-bytes", func(args ...any) any {
		rs := asReadable("-stream-read-bytes", args, 2)
		return rs.readBytes(asInt(args[1]))
	})
	// -stream-read-line (readable) -> line string (no trailing newline), or nil at EOF.
	def("-stream-read-line", func(args ...any) any {
		return asReadable("-stream-read-line", args, 1).readLine()
	})
	// -stream-read-all (readable) -> the rest of the stream as one string.
	def("-stream-read-all", func(args ...any) any {
		return asReadable("-stream-read-all", args, 1).readAll()
	})
	// -stream-chunk-seq (readable n) -> lazy seq of []byte chunks of size n.
	def("-stream-chunk-seq", func(args ...any) any {
		return asReadable("-stream-chunk-seq", args, 2).chunkSeq(asInt(args[1]))
	})
	// -stream-line-seq (readable) -> lazy seq of line strings.
	def("-stream-line-seq", func(args ...any) any {
		return asReadable("-stream-line-seq", args, 1).lineSeq()
	})
	// -stream-write (writable data) -> nil (buffered write + flush).
	def("-stream-write", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-stream-write expects 2 args (writable data), got %d", len(args)))
		}
		asWritable("-stream-write", args[0]).write(args[1])
		return nil
	})
	// -stream-of-file (path) -> a ReadableStream over the file at path (ADR
	// 0110 ask 1). The ONE stream abstraction now covers files too, not just
	// process pipes and sockets: the handle is the same seqable readable, so
	// lines/chunks/reduce/transducers walk a file in constant memory. The
	// caller closes it.
	def("-stream-of-file", func(args ...any) any {
		path := asString(one("-stream-of-file", args))
		f, err := os.Open(path)
		if err != nil {
			panic(fmt.Errorf("cljg.stream/of-file: cannot open %s: %w", path, err))
		}
		return newReadableStream(f, f)
	})
	// -stream-to-file (path append?) -> a WritableStream over the file at
	// path, truncating it unless append? is truthy. close flushes and closes.
	def("-stream-to-file", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -stream-to-file (expects 2: [path append?])", len(args)))
		}
		path := asString(args[0])
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if isTruthy(args[1]) {
			flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		}
		f, err := os.OpenFile(path, flags, 0o644)
		if err != nil {
			panic(fmt.Errorf("cljg.stream/to-file: cannot open %s: %w", path, err))
		}
		return newWritableStream(f, f)
	})
	// -stream-close (stream) -> nil. Works on either a readable or a writable.
	def("-stream-close", func(args ...any) any {
		switch s := one("-stream-close", args).(type) {
		case *ReadableStream:
			s.closeRead()
		case *WritableStream:
			s.closeWrite()
		default:
			panic(fmt.Errorf("cljg.stream: close expects a stream handle, got: %s", lang.PrintString(args[0])))
		}
		return nil
	})
}

// asReadable coerces args[0] to a *ReadableStream, checking arity.
func asReadable(name string, args []any, arity int) *ReadableStream {
	if len(args) != arity {
		panic(fmt.Errorf("%s expects %d args, got %d", name, arity, len(args)))
	}
	rs, ok := args[0].(*ReadableStream)
	if !ok {
		panic(fmt.Errorf("cljg.stream: %s expects a readable stream, got: %s", name, lang.PrintString(args[0])))
	}
	return rs
}

// asWritable coerces v to a *WritableStream.
func asWritable(name string, v any) *WritableStream {
	ws, ok := v.(*WritableStream)
	if !ok {
		panic(fmt.Errorf("cljg.stream: %s expects a writable stream, got: %s", name, lang.PrintString(v)))
	}
	return ws
}
