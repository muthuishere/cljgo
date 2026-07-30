# s53 — clojure.data.json, natively (ADR 0096)

Native cljgo port of **org.clojure/data.json 2.5.1** (single upstream file
`clojure/data/json.clj`, EPL-1.0, Stuart Sierra). Draft only — nothing here is
wired into `core/` or `pkg/` this round.

## Starting point (from S50)

S50 measured this library at **1 ns, ~100% Java**: the reader is a custom Java
`PushbackReader`/`StringReader` (`ReaderPBR`/`StringPBR` over `java.io`), the
writer a `StringBuilder`/`StringWriter` + `java.io.Writer` driven by a
`defprotocol JSONWriter` that `extend`s ~20 concrete Java classes
(`Long`, `Double`, `BigInteger`, `UUID`, `Instant`, `java.util.Map`, …). None
of that survives on cljgo, which has no Java class hierarchy. The guts had to
be rewritten; the semantics did not.

## Confirmed strategy

**Faithful native rewrite of the reader/writer guts; byte-identical public
surface.**

- **Reader** — the `InternalPBR` pushback stream is replaced by a pure-Clojure
  char cursor over a String: `{:s :len :pos (atom int)}` with `read-char`
  (returns the int code or `-1`) and `unread-char` (backs up one). The entire
  number state machine (`:minus → :int-zero → :int-digit → :frac-* → :exp-*`)
  is preserved verbatim, so the same inputs are accepted/rejected. The
  array-buffer fast path (`char-array`/`readChars`/`unreadChars`) is dropped;
  the correctness-equivalent slow path is always taken.
- **Writer** — the `JSONWriter` protocol becomes a single `-write` `cond` over
  cljgo predicates (`nil?`/`boolean`/`keyword?`+`symbol?`/`string?`/`ratio?`/
  `float?`/`number?`/`map?`/seqable). The `Appendable`/`StringBuilder` becomes
  an atom holding a transient vector of string parts, joined with `apply str`.
  The `codepoint-decoder` short-table classification, `->hex-string` padding,
  `write-indent`, and the key-fn/value-fn/`:escape-*` option handling are all
  reproduced.
- **Numbers** — `Long/valueOf`+`bigint` fallback → `parse-long` else `bigint`;
  `Double/valueOf` → `parse-double`; `bigdec` unchanged; hex escape parsing
  (`Integer/parseInt … 16`) is a hand-written `hex-digit` helper. Double string
  formatting matches the oracle exactly (`0.3333333333333333`, `-1250.0`).

## Namespace split — pure vs Java

There is a single upstream namespace, `clojure.data.json`. The split is by
FEATURE inside it:

| upstream feature | disposition |
| --- | --- |
| `read-str`, `read-json` (String input), the whole parser | **ports ~verbatim** (Java stream → pure cursor) |
| `write-str`, `json-str`, escaping/indent/key-fn/value-fn | **ports ~verbatim** (protocol → `cond`, StringBuilder → transient vec) |
| number parse/format, unicode escape, error strings | **ports** with pure-Clojure equivalents; byte-identical output |
| `read`/`write` against a real `java.io.Reader`/`Writer` | **rewritten to String-only**; the Reader/Writer arms need a Go host (`cljg.io`) — scoped out, stubbed to delegate/return the string |
| `:extra-data-fn` (`on-extra-throw*`, `toReader`) | **scoped out** — needs a live Reader tail |
| `pprint`, `pprint-json`, `print-json`, `write-json` | **scoped out** — need `clojure.pprint`/`*out*`/host Writer |
| JSONWriter for `UUID`/`Instant`/`Date`/`sql.Date`/`Float`/`Ratio`Java-specifics | Ratio ported (→ double); temporal/UUID **scoped out** (no host types) |

## What ports verbatim vs what was rewritten

- **Verbatim in spirit** (same control flow, same names, same error strings):
  the number state machine, `read-object`/`read-array`/`read-key`, escape
  decoding, `write-object`/`write-array`/`write-indent`/`write-string`, the
  key-fn/value-fn contract (including "value-fn returns itself ⇒ omit").
- **Rewritten** (Java capability replaced): the pushback stream, the output
  accumulator, the protocol dispatch, and every `Java-static`/`Java-class`
  touchpoint.

## Verification

`cljgo` does not implement `load-file`, so the port was verified by inlining
`draft-data_json.cljg` ahead of a driver (the satellite opens with
`(clojure.core/in-ns 'clojure.data.json)`, which works inline) and running it
with `cljgo run`. **Every** frozen behavior in
`draft-conformance-data.json.clj` reproduces the JVM oracle byte-for-byte,
including the four error strings. See VERDICT.md for the residual unknowns
(chiefly astral-plane `\uXXXX` surrogate handling).

## Files

- `draft-data_json.cljg` — the port (satellite shape, EPL header preserved).
- `draft-conformance-data.json.clj` — 15 oracle-frozen behaviors.
- `VERDICT.md` — MET/SCOPED call, honest tier-1 scope, integration notes.
