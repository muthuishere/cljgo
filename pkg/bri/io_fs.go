// io_fs.go — the Go half of cljg.io's filesystem surface (ADR 0089): the
// structural file/path/directory ops clojure.core lacks (slurp/spit stay core).
// Thin shims over stdlib os + path/filepath — pure Go, so CGO_ENABLED=0 +
// cljgo dist hold, and a non-OptIn namespace (no dependency to isolate). The
// ergonomic API (exists?/mkdirs/copy!/path/…) is portable Clojure
// (core/cljg/io.cljg). Interned as :private vars into cljg.io.
//
// cljg.io rides the same name-generic embedded-namespace registry as bri and
// cljg.net.http / cljg.os (the pkg/bri package name is a legacy of bri being the
// first tenant — ADR 0087 §1).
package bri

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// installIOShims interns cljg.io's private Go filesystem primitives.
func installIOShims(def func(name string, fn func(args ...any) any)) {
	// -fs-stat path -> {:dir? :file? :size :modified} or nil when absent.
	def("-fs-stat", func(args ...any) any {
		fi, err := os.Stat(asString(one("-fs-stat", args)))
		if err != nil {
			return nil // absent (or unreadable) is nil, not an error — callers ask exists?/size
		}
		return lang.NewMap(
			lang.NewKeyword("dir?"), fi.IsDir(),
			lang.NewKeyword("file?"), fi.Mode().IsRegular(),
			lang.NewKeyword("size"), fi.Size(),
			lang.NewKeyword("modified"), fi.ModTime().UnixMilli(),
		)
	})
	// -fs-list path -> vector of ENTRY NAMES (not full paths), sorted.
	def("-fs-list", func(args ...any) any {
		ents, err := os.ReadDir(asString(one("-fs-list", args)))
		if err != nil {
			panic(fmt.Errorf("cljg.io: list %q: %w", one("-fs-list", args), err))
		}
		out := make([]any, len(ents))
		for i, e := range ents {
			out[i] = e.Name()
		}
		return lang.NewVectorOwning(out)
	})
	// -fs-mkdir path -> nil (mkdir -p: creates parents, no error if it exists).
	def("-fs-mkdir", func(args ...any) any {
		p := asString(one("-fs-mkdir", args))
		if err := os.MkdirAll(p, 0o755); err != nil {
			panic(lang.NewIOError("cljg.io/mkdirs", lang.NewKeyword("fs/mkdir"), p, err))
		}
		return nil
	})
	// -fs-delete (path recursive?) -> nil. recursive? true removes a tree;
	// false removes a single file/empty dir. Missing path is not an error.
	def("-fs-delete", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-fs-delete expects 2 args (path recursive?), got %d", len(args)))
		}
		p := asString(args[0])
		var err error
		op := "cljg.io/delete!"
		if args[1] != nil && args[1] != false {
			op = "cljg.io/delete-tree!"
			err = os.RemoveAll(p)
		} else {
			err = os.Remove(p)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			panic(lang.NewIOError(op, lang.NewKeyword("fs/delete"), p, err))
		}
		return nil
	})
	// -fs-copy (src dst) -> nil. Copies file contents + mode (not a tree).
	def("-fs-copy", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-fs-copy expects 2 args (src dst), got %d", len(args)))
		}
		src, dst := asString(args[0]), asString(args[1])
		if err := copyFile(src, dst); err != nil {
			panic(fmt.Errorf("cljg.io: copy %q -> %q: %w", src, dst, err))
		}
		return nil
	})
	// -fs-move (src dst) -> nil. Rename, falling back to copy+delete across
	// filesystems (os.Rename fails with EXDEV on a cross-device move).
	def("-fs-move", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-fs-move expects 2 args (src dst), got %d", len(args)))
		}
		src, dst := asString(args[0]), asString(args[1])
		if err := os.Rename(src, dst); err != nil {
			if copyErr := copyFile(src, dst); copyErr != nil {
				panic(fmt.Errorf("cljg.io: move %q -> %q: %w", src, dst, err))
			}
			if rmErr := os.Remove(src); rmErr != nil {
				panic(fmt.Errorf("cljg.io: move %q -> %q (removing src): %w", src, dst, rmErr))
			}
		}
		return nil
	})
	// -fs-glob pattern -> vector of matching paths (filepath.Glob semantics), sorted.
	def("-fs-glob", func(args ...any) any {
		pat := asString(one("-fs-glob", args))
		m, err := filepath.Glob(pat)
		if err != nil {
			panic(fmt.Errorf("cljg.io: glob %q: %w", pat, err))
		}
		out := make([]any, len(m))
		for i, p := range m {
			out[i] = p
		}
		return lang.NewVectorOwning(out)
	})
	// -fs-walk root -> vector of every path under root (root included), depth-first.
	def("-fs-walk", func(args ...any) any {
		root := asString(one("-fs-walk", args))
		var out []any
		err := filepath.Walk(root, func(p string, _ os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			out = append(out, p)
			return nil
		})
		if err != nil {
			panic(fmt.Errorf("cljg.io: walk %q: %w", root, err))
		}
		return lang.NewVectorOwning(out)
	})
	// -fs-temp-file (prefix suffix) -> path of a freshly created temp file.
	def("-fs-temp-file", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-fs-temp-file expects 2 args (prefix suffix), got %d", len(args)))
		}
		f, err := os.CreateTemp("", asString(args[0])+"*"+asString(args[1]))
		if err != nil {
			panic(fmt.Errorf("cljg.io: temp-file: %w", err))
		}
		name := f.Name()
		f.Close()
		return name
	})
	// -fs-temp-dir prefix -> path of a freshly created temp directory.
	def("-fs-temp-dir", func(args ...any) any {
		d, err := os.MkdirTemp("", asString(one("-fs-temp-dir", args)))
		if err != nil {
			panic(fmt.Errorf("cljg.io: temp-dir: %w", err))
		}
		return d
	})
	// -fs-home -> the current user's home directory.
	def("-fs-home", func(args ...any) any {
		h, err := os.UserHomeDir()
		if err != nil {
			panic(fmt.Errorf("cljg.io: home: %w", err))
		}
		return h
	})
	// -fs-cwd -> the current working directory.
	def("-fs-cwd", func(args ...any) any {
		wd, err := os.Getwd()
		if err != nil {
			panic(fmt.Errorf("cljg.io: cwd: %w", err))
		}
		return wd
	})

	// --- path math (host filepath semantics) ---------------------------------
	// -path-abs path -> absolute, cleaned path (resolved against cwd).
	def("-path-abs", func(args ...any) any {
		abs, err := filepath.Abs(asString(one("-path-abs", args)))
		if err != nil {
			panic(fmt.Errorf("cljg.io: absolute: %w", err))
		}
		return abs
	})
	// -path-real path -> absolute, cleaned path with every symlink resolved.
	// Unlike -path-abs this touches the filesystem: EvalSymlinks stats every
	// path component, so a non-existent path or a symlink cycle is a real
	// error (ELOOP), not a value to fake. Coded G5024 (ADR 0089 §"real-path").
	def("-path-real", func(args ...any) any {
		p := asString(one("-path-real", args))
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			panic(lang.NewCodedError("G5024",
				fmt.Sprintf("cljg.io/real-path: cannot resolve %q: %v", p, err)))
		}
		abs, err := filepath.Abs(real)
		if err != nil {
			panic(lang.NewCodedError("G5024",
				fmt.Sprintf("cljg.io/real-path: cannot resolve %q: %v", p, err)))
		}
		return abs
	})
	// -path-join [segments…] -> the segments joined + cleaned with the host separator.
	def("-path-join", func(args ...any) any {
		segs := toStringSlice(one("-path-join", args))
		return filepath.Join(segs...)
	})
	// -path-base path -> the final element (filename).
	def("-path-base", func(args ...any) any { return filepath.Base(asString(one("-path-base", args))) })
	// -path-dir path -> everything but the final element (parent).
	def("-path-dir", func(args ...any) any { return filepath.Dir(asString(one("-path-dir", args))) })
	// -path-ext path -> the extension including the dot ("" if none).
	def("-path-ext", func(args ...any) any { return filepath.Ext(asString(one("-path-ext", args))) })

	// --- byte-level whole-file I/O (ADR 0110 ask 1) --------------------------
	// The route between a path and BYTES that neither slurp nor spit can give:
	// they go through a string, and a string round-trip is lossy for non-UTF-8
	// content (invalid sequences become U+FFFD) on this host exactly as it is
	// on the JVM. These two read/write the raw bytes.
	//
	// -fs-read-bytes path -> the whole file as a byte-array ([]byte).
	def("-fs-read-bytes", func(args ...any) any {
		path := asString(one("-fs-read-bytes", args))
		b, err := os.ReadFile(path)
		if err != nil {
			panic(lang.NewIOError("cljg.io/read-bytes", lang.NewKeyword("fs/read"), path, err))
		}
		return toClojureBytes(b)
	})
	// -fs-write-bytes (path data opts) -> the number of bytes written.
	// data is a byte-array or a string; opts is nil or a map ({:append true}
	// appends, anything else truncates). opts is validated HERE rather than
	// destructured in Clojure so a non-map (e.g. the bare `true` a caller
	// reaches for) is REJECTED by name instead of silently truncating.
	def("-fs-write-bytes", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -fs-write-bytes (expects 3: [path data opts])", len(args)))
		}
		path := asString(args[0])
		data := toGoBytes("cljg.io/write-bytes", args[1])
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if isTruthy(optsAppend("cljg.io/write-bytes", args[2])) {
			flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		}
		f, err := os.OpenFile(path, flags, 0o644)
		if err != nil {
			panic(fmt.Errorf("cljg.io/write-bytes: cannot open %s: %w", path, err))
		}
		defer f.Close()
		n, err := f.Write(data)
		if err != nil {
			panic(fmt.Errorf("cljg.io/write-bytes: cannot write %s: %w", path, err))
		}
		return int64(n)
	})

	installProcShims(def) // cljg.io also owns process exec (io_proc.go)
}

// isTruthy is Clojure truthiness (everything but nil and false).
func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

// optsAppend reads :append out of an options argument, REJECTING anything that
// is not nil or a map. The trailing opts of write-bytes / to-file is a map, and
// a caller who writes the obvious (write-bytes p data true) must be told so:
// destructuring `(:append true)` in Clojure yields nil, which silently
// TRUNCATES the file the caller asked to append to — the worst kind of wrong.
// Coded G5017 so `cljgo explain` has something to say.
func optsAppend(name string, v any) any {
	switch m := v.(type) {
	case nil:
		return nil
	case lang.IPersistentMap:
		return lang.Get(m, lang.NewKeyword("append"))
	default:
		panic(&lang.CodedError{
			Code: "G5017",
			Msg: fmt.Sprintf("%s: options must be a map (expects {:append true}, found: %s)",
				name, lang.PrintString(v)),
		})
	}
}

// toGoBytes coerces a Clojure byte payload to Go bytes: a string (its UTF-8
// bytes), a Go-native []byte (what raw Go interop hands back), or the []int8
// clojure.core/byte-array builds and every cljg byte producer returns — all
// three answer true to `bytes?`/`string?`, so all three are accepted wherever
// a byte payload is asked for. name is the PUBLIC fn name, so the message
// points at what the caller wrote. THE RULE (ADR 0110): every cljg byte
// CONSUMER goes through here and every cljg byte PRODUCER goes through
// toClojureBytes, so a producer's output always feeds a consumer's input.
func toGoBytes(name string, v any) []byte {
	switch b := v.(type) {
	case string:
		return []byte(b)
	case []byte:
		return b
	case []int8:
		out := make([]byte, len(b))
		for i, c := range b {
			out[i] = byte(c)
		}
		return out
	default:
		panic(fmt.Errorf("%s: expected a byte-array or string, found: %s", name, lang.PrintString(v)))
	}
}

// toClojureBytes wraps Go bytes as the SIGNED []int8 clojure.core/byte-array
// builds, which is what the JVM's byte[] is: (vec (Files/readAllBytes p)) over
// a 0xFF byte is [-1] on clojure 1.12.5, not [255] (oracle, 2026-07-30). EVERY
// cljg byte producer returns this — cljg.io/read-bytes,
// cljg.security/base64-decode-bytes, cljg.stream/read-bytes + chunks,
// cljg.compress/gzip + gunzip — so ONE representation reaches user code from
// every route, its elements read the same on both hosts, and it feeds straight
// back into write-bytes / gunzip / sha256 / aget / alength / bytes?.
func toClojureBytes(b []byte) []int8 {
	out := make([]int8, len(b))
	for i, c := range b {
		out[i] = int8(c)
	}
	return out
}

// copyFile copies src to dst, preserving the source file mode.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// toStringSlice coerces a cljgo sequential (vector/seq) of strings to []string.
func toStringSlice(v any) []string {
	var out []string
	for s := lang.Seq(v); s != nil; s = lang.Next(s) {
		out = append(out, asString(lang.First(s)))
	}
	return out
}
