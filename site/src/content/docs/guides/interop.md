---
title: Zero-binding Go interop
description: Call any Go package from Clojure with require-go — no wrappers, no generated stubs. Errors as values, the ! unwrap suffix, methods, fields, and struct constructors.
---

Interop is cljgo's #1 design goal: the Go ecosystem is the standard
library. `require-go` pulls in a Go package and you call it directly — no
bindings, no wrappers, no generated stubs. The same forms run identically
interpreted and compiled (that parity is conformance-tested; see
[the REPL guide](/cljgo/guides/repl/)).

## Packages are namespaces

```clojure
(require-go '[strings])
(require-go '[math])

(strings/ToUpper "hi")        ; => "HI"
(strings/Repeat "ab" 3)       ; => "ababab" (int64 args coerce to Go int)
math/Pi                       ; consts and vars work in value position
```

Aliases work the way you expect: `(require-go '[net/url])` gives you a
`url/` prefix; `(require-go '[net/http :as http])` gives you `http/`.

Go's export rule is part of the surface — you call `strings/ToUpper`,
not `strings/to-upper`. cljgo does not rename or re-case Go identifiers.

## Errors are values; `!` unwraps

A Go function returning `(T, error)` shapes to a `[v err]` vector. The
error slot is nil-normalized, so it is directly truthy-testable:

```clojure
(require-go '[strconv])

(strconv/Atoi "123")                              ; => [123 nil]
(first (strconv/Atoi "x"))                        ; => 0 (Go's zero value passes through)
(if (get (strconv/Atoi "x") 1) :parse-error :ok)  ; => :parse-error
```

Appending `!` to the call unwraps the value or throws — the 90% path.
`!` can never appear in a Go identifier, so the suffix is unambiguous
sugar:

```clojure
(strconv/Atoi! "42")             ; => 42
(strconv/Atoi! "not-a-number")   ; throws, carrying the Go error
```

Both shapes are frozen in the conformance suite
(`conformance/tests/interop-verr.clj`, `interop-bang-throw.clj`).

## Methods, fields, constructors

Clojure's dot forms, verbatim:

```clojure
(require-go '[net/url])

;; construct: (pkg/T. {:Field v}) builds a *url.URL — &url.URL{...}
(def u (url/URL. {:Scheme "https" :Host "x"}))

;; field write: (set! (.-Field r) v)
(set! (.-Host u) "y")

;; zero-valued pointer: (go/new pkg/T) — new(url.URL)
(def z (go/new url/URL))

[(.-Scheme u) (.-Host u) (.-Path z)]   ; => ["https" "y" ""]
```

```clojure
(require-go '[strings])

(def r (strings/NewReplacer "a" "1" "b" "2"))
(.Replace r "abcab")    ; method call => r.Replace("abcab") => "12c12"
```

Field reads use `(.-Field r)` — `(url/Parse! "https://example.com/a/b")`
followed by `(.-Host u)` gives `"example.com"`
(`conformance/tests/interop-field.clj`). Method and field access resolves
reflectively at runtime in both modes today (receiver-typed direct
emission is later roadmap work), so behavior is byte-identical by
construction.

## Third-party modules

Any module on the Go proxy links with zero hand-written bindings. Declare
it in your project's `build.cljgo` — cljgo synthesizes the `go.mod`, runs
`go get`, and the emitter resolves calls from real `go/packages` type
facts (this is `examples/build-websocket` in the repo):

```clojure
;; build.cljgo
(defn build [b]
  (let [app (exe b {:name "wsclient" :main "src/main.cljg"})]
    (go-require app "github.com/gorilla/websocket" "v1.5.3")
    (install b app)
    (run b app)))
```

```clojure
;; src/main.cljg
(require-go '["github.com/gorilla/websocket" :as ws])
(println "close-normal code:" ws/CloseNormalClosure)
(def frame (ws/FormatCloseMessage ws/CloseNormalClosure "bye"))
```

**Honest limit:** the interpreter (`cljgo repl` / `cljgo run`) can only
see Go packages compiled into it — the stdlib works everywhere, but a
third-party module is reachable only in a compiled build. The
interpreted leg **hard-errors** on an unlinked package rather than
silently returning `nil` — never a wrong answer, always a loud one
(ADR 0053):

```
$ cljgo run tp.clj
error: go module github.com/google/uuid is not linked into the
  interpreter (accessing member NewString) (at tp.clj); build it (cljgo build)
help: run `cljgo explain G5000`
```

cgo-based Go modules (sqlite drivers, sensors, GUI/audio) import like
any other module — they are just Go modules — though they constrain
cross-compilation (see [Compile & ship](/cljgo/guides/compile/)).

## Not supported (yet, or ever)

- **Java interop — never.** cljgo compiles to Go, not JVM bytecode.
  JVM-surface forms like `(System/getProperty …)` or `import` fail
  loudly at analysis with a named error, not silently.
- **C FFI (`ffi/deflib` on purego)** — designed (ADR 0044) but **not
  implemented**. Today C is reached via cgo-based Go modules.
- **Go generics** — no `go/instantiate` form exists yet. Among the `go/`
  interop operators, only `go/new` is implemented.
- **ns-form `:require-go`** — use the function form `(require-go '[...])`;
  that is what every shipped example and conformance test uses.

Channels received from Go APIs participate in core.async directly — see
[Concurrency & core.async](/cljgo/guides/concurrency/).
