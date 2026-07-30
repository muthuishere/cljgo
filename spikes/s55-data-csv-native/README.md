# Spike s55 — `clojure.data.csv`, natively (ADR 0096 tier-1)

**Question (ADR 0096 §Spikes):** does the `clojure.data.csv` reader/writer port
to cljgo with oracle-matching quoting / separator / quote-char / newline
behavior?

**Answer: YES — MET.** The whole library (both `read-csv` and `write-csv`, all
option keys) ports to pure Clojure for String input, and all 20 probed behaviors
reproduce the JVM oracle byte-for-byte on cljgo. See `VERDICT.md`.

## Upstream

`org.clojure/data.csv` **1.1.0**, single source file `clojure/data/csv.clj`
(Jonas Enlund, EPL-1.0). Resolved via the real Clojure CLI:

```
clojure -Sdeps '{:deps {org.clojure/data.csv {:mvn/version "1.1.0"}}}' -Spath
# -> ~/.m2/repository/org/clojure/data.csv/1.1.0/data.csv-1.1.0.jar
```

147 lines. Reader: a char-code state machine over a `java.io.PushbackReader`
(one char of pushback for CR/LF folding), cells accumulated in a
`StringBuilder`, dispatched by a `Read-CSV-From` protocol over
`String`/`Reader`/`PushbackReader`. Writer: emits to a `java.io.Writer`, using
`clojure.string/escape` to double the quote char.

## Pure / Java split (the whole file is one namespace)

`clojure.data.csv` is a **single namespace**. There is no pure-vs-Java namespace
split; the split is *within* the file:

| upstream piece | JVM dependency | cljgo port |
|---|---|---|
| `read-quoted-cell` / `read-cell` / `read-record` | `StringBuilder`, char codes | **pure Clojure**, string accumulation — same `condp ==` state machine, verbatim in shape |
| the `Read-CSV-From` protocol (String / Reader / PushbackReader) | `PushbackReader`, `StringReader` | **replaced** by a pure one-slot-pushback string reader (`pb-reader` / `rd` / `unrd`) for **String** input; streaming `Reader` input is scoped out (needs a host char-reader) |
| `read-csv` public API | — | **verbatim** arglist + option keys (`:separator` `:quote`) |
| `write-cell` / `write-record` / `write-csv*` | `java.io.Writer.write` | **pure Clojure** emitting to `*out*` via `print` |
| `write-csv` public API | `java.io.Writer` first arg | **verbatim** arglist + option keys (`:separator` `:quote` `:quote?` `:newline`); `writer` is bound to `*out*` |
| `clojure.string/escape` (quote-doubling) | — | reused as-is (cljgo ships `clojure.string`, oracle-verified) |

### Ports ~verbatim
The entire read state machine (`lf`/`cr`/`eof` sentinels, `:sep`/`:eol`/`:eof`
dispatch, CRLF folding, bare-CR-as-EOL, doubled-quote un-escape, the
`(= record [""])` empty-tail suppression) and the entire write path
(quote-when-necessary predicate `#{separator quote \return \newline}`,
`str/escape` doubling, `:lf`/`:cr+lf` newline map, `str`-coercion of non-string
cells) are ported line-for-line in shape.

### Rewritten
- **`PushbackReader` -> `pb-reader`**: a `(atom {:s :n :i :pb})` with `rd`
  (next char code or `-1`, advance) and `unrd` (push one code back). Faithfully
  mirrors a size-1 `PushbackReader`, including pushing back `eof`. `(nil? pb)`
  (not `(if pb)`) tests emptiness because char code `0` is truthy in Clojure.
- **`StringBuilder` -> immutable string accumulation** (`(str acc (char ch))`).
- **`java.io.Writer` -> `*out*`**: `write-csv` does `(binding [*out* writer] …)`
  and emits with `print`. The idiomatic sink is
  `(with-out-str (write-csv *out* data …))`, which yields the exact bytes the
  JVM `StringWriter` path did.

## What does NOT port (honest scope)
- **Streaming `java.io.Reader` input** to `read-csv` (non-String). Needs a host
  streaming char-reader (a small new `cljg.io` host fn). String input — the
  common case — is fully native. `read-csv` throws a clear error for non-String
  input instead of silently diverging.
- **A real streaming `Writer` sink** (file / socket) for `write-csv`. Any sink
  bindable to `*out*` works (that is how `with-out-str` and cljgo's file-output
  paths already operate); a first-class `java.io.Writer` equivalent is a host
  concern, not this port's.
- **Error-message bytes** on malformed input. cljgo has no `java.io.EOFException`
  and no `%c` `format`; the port raises `ex-info` with a close paraphrase. Happy
  paths are byte-frozen; the malformed-input message text is not.

## Verification

The port was inlined after its `(in-ns 'clojure.data.csv)` header and run on
cljgo (`~/.local/bin/cljgo run`) against a 20-case driver (11 read + 9 write).
Every case matched the JVM oracle **byte-for-byte** (diff clean). `load-file`
is not callable in this cljgo build, so the spike verifies by inlining the
source in one file — the loader (`pkg/eval`) is the integration path, not
`load-file`.

## Files
- `draft-data_csv.cljg` — the native port (EPL header preserved).
- `draft-conformance-data.csv.clj` — 14 oracle-frozen behaviors.
- `VERDICT.md` — MET, scope, integration steps, residual unknowns.
