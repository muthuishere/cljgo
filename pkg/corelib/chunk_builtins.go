package corelib

import (
	"fmt"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// chunk_builtins.go — clojure.core's chunk API compat shims
// (fundamentals batch A4, 2026-07-23): chunk / chunk-append /
// chunk-buffer / chunk-cons / chunk-first / chunk-next / chunk-rest /
// chunked-seq?, exposed over cljgo's REAL chunk machinery
// (lang.ChunkBuffer / lang.ChunkedCons / lang.IChunkedSeq, ADR 0063).
// Libraries (core.async's own source, transit) call these by name.
// Registered into internBuiltins by ONE line (internChunkBuiltins(def)),
// per the merge-friendly discipline.
//
// Every behavior oracle-verified vs JVM Clojure 1.12.5 (2026-07-23);
// frozen evidence in conformance/tests/chunk-api-*.clj.
//
// Documented DEVIATION: on the JVM a persistent vector's seq is chunked
// ((chunked-seq? (seq [1 2 3])) => true); cljgo's vector seq (apvSeq)
// is not chunked yet, so that expression is false here. Ranges and
// lazy map/filter/keep output ARE chunked in both worlds.

func internChunkBuiltins(def func(name string, fn func(args ...any) any) *lang.Var) {
	// (chunk-buffer capacity) -> a fresh ChunkBuffer. Following Clojure,
	// the buffer is fixed-capacity: appending beyond it is an error.
	def("chunk-buffer", func(args ...any) any {
		return lang.NewChunkBuffer(int(lang.AsInt64(oneArg("chunk-buffer", args))))
	})

	// (chunk-append b x) -> nil; adds x to the buffer.
	def("chunk-append", func(args ...any) any {
		b, x := twoArgs("chunk-append", args)
		cb, ok := b.(*lang.ChunkBuffer)
		if !ok {
			panic(fmt.Errorf("chunk-append: not a chunk buffer: %s", lang.PrintString(b)))
		}
		cb.Add(x)
		return nil
	})

	// (chunk b) -> the buffer's contents as an IChunk (the buffer is
	// consumed, as on the JVM). oracle: (let [b (chunk-buffer 4)]
	// (chunk-append b 1) (chunk-append b 2) (count (chunk b))) => 2.
	def("chunk", func(args ...any) any {
		cb, ok := oneArg("chunk", args).(*lang.ChunkBuffer)
		if !ok {
			panic(fmt.Errorf("chunk: not a chunk buffer: %s", lang.PrintString(args[0])))
		}
		return cb.Chunk()
	})

	// (chunk-first s) -> the chunked seq's first chunk.
	// oracle: (count (chunk-first (seq (range 100)))) => 32.
	def("chunk-first", func(args ...any) any {
		return chunkedSeqArg("chunk-first", oneArg("chunk-first", args)).ChunkedFirst()
	})

	// (chunk-rest s) -> the seq past the first chunk (may be a lazy seq).
	// oracle: (first (chunk-rest (seq (range 100)))) => 32.
	def("chunk-rest", func(args ...any) any {
		return chunkedSeqArg("chunk-rest", oneArg("chunk-rest", args)).ChunkedMore()
	})

	// (chunk-next s) -> (seq (chunk-rest s)).
	// oracle: (first (chunk-next (seq (range 100)))) => 32.
	def("chunk-next", func(args ...any) any {
		return chunkedSeqArg("chunk-next", oneArg("chunk-next", args)).ChunkedNext()
	})

	// (chunk-cons chunk rest) -> rest when the chunk is empty, else a
	// ChunkedCons of chunk onto rest — exactly clojure.core/chunk-cons.
	// oracle: (chunk-cons <chunk of 1 2> '(9 9)) => (1 2 9 9);
	// (chunk-cons <empty chunk> '(7 8)) => (7 8).
	def("chunk-cons", func(args ...any) any {
		c, rest := twoArgs("chunk-cons", args)
		chunk, ok := c.(lang.IChunk)
		if !ok {
			panic(fmt.Errorf("chunk-cons: not a chunk: %s", lang.PrintString(c)))
		}
		if chunk.Count() == 0 {
			return rest
		}
		return lang.NewChunkedCons(chunk, lang.Seq(rest))
	})

	// (chunked-seq? s) -> is s a chunked seq?
	// oracle: (chunked-seq? (seq (range 100))) => true;
	// (chunked-seq? (seq '(1 2 3))) => false.
	def("chunked-seq?", func(args ...any) any {
		_, ok := oneArg("chunked-seq?", args).(lang.IChunkedSeq)
		return ok
	})
}

// chunkedSeqArg coerces a chunk-first/-rest/-next argument to an
// IChunkedSeq, with the fn named in the miss (error-message doctrine).
func chunkedSeqArg(op string, v any) lang.IChunkedSeq {
	cs, ok := v.(lang.IChunkedSeq)
	if !ok {
		panic(fmt.Errorf("%s: not a chunked seq: %s", op, lang.PrintString(v)))
	}
	return cs
}
