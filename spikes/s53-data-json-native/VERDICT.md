# s53 VERDICT — clojure.data.json native port

**Verdict: SCOPED (per ADR 0027).** A faithful native port lands in tier-1 for
the entire String-in / String-out surface — `read-str`, `read-json`,
`write-str`, `json-str`, and `read`/`write` reduced to their String forms —
byte-identical to the JVM oracle across all 15 frozen behaviors, including the
four error strings. The Reader/Writer streaming arms, `pprint`, and the Java
temporal/UUID writers are deferred behind host capabilities cljgo does not yet
have. One genuine semantic residual (astral-plane `\uXXXX`) is documented below.

## What is MET (ships in tier-1 now)

- `read-str` — full parser: objects, arrays, strings with all escapes
  (`\" \\ \/ \b \f \n \r \t \uXXXX`), the complete number state machine
  (Long / BigInt promotion / Double / `:bigdec` BigDecimal), `true`/`false`/
  `null`, and options `:key-fn`, `:value-fn` (incl. "returns itself ⇒ omit"),
  `:bigdec`, `:eof-error?`, `:eof-value`. Error strings byte-identical.
- `write-str` — full generator: `nil`/booleans/keywords/symbols/strings/
  integers/BigInt/Double/Ratio(→double)/maps/vectors/lists/seqs/sets, with
  `:escape-unicode`, `:escape-js-separators`, `:escape-slash`, `:key-fn`,
  `:value-fn`, `:indent`, `:default-write-fn`. Escaping, indentation, and NaN/
  Inf rejection match the oracle.
- `read-json`, `json-str` (deprecated but string-based) — ported.
- `read`/`write` — kept in the public API; delegate to the String path
  (`write` returns the JSON string; `read` requires a String and says so
  plainly otherwise).

## What is SCOPED OUT (and why)

| feature | blocker | path to full |
| --- | --- | --- |
| `read` from a live `java.io.Reader`, `write` to a `java.io.Writer` | cljgo has no host Reader/Writer | add `cljg.io` host reader/writer, then restore the Reader/Writer arms |
| `:extra-data-fn`, `on-extra-throw`, `on-extra-throw-remaining` | need a live Reader tail (`toReader` + `slurp`) | same `cljg.io` host reader |
| `pprint`, `pprint-json` | need `clojure.pprint` / `cl-format` | ships when cljgo grows `clojure.pprint` |
| `print-json`, `write-json` | write to `*out*` / a host Writer | `cljg.io` host writer |
| JSONWriter for `UUID` / `Instant` / `Date` / `sql.Date` | no host temporal/UUID types in cljgo | add host types + extend the `-write` `cond` (or a real protocol) |

## Residual unknown — astral-plane characters (the one real divergence)

cljgo strings are **rune**-indexed; the JVM is **UTF-16 code-unit**-indexed.
For every character in the BMP (`< U+10000`) the two are identical, and all 15
frozen behaviors are BMP. Outside the BMP they diverge (verified, not guessed):

```
(write-str "😀")            JVM => "😀"   cljgo port => "ὠ0"
(read-str "\"😀\"") JVM => "😀"            cljgo port => two loose surrogates
```

The port emits a single `ὠ0` (5 hex digits) instead of the surrogate pair,
and does not recombine a surrogate pair on read. Closing this needs either a
UTF-16 surrogate layer inside the pure-Clojure writer/reader, or a small Go
host helper. It should be called out in the ADR/spec as a known limitation for
the first cut (emoji/rare-CJK in JSON strings), NOT silently shipped as
"identical".

## Go host needs

- **None required for the tier-1 MET surface** — the String path is pure
  Clojure over existing core (`parse-long`, `parse-double`, `bigint`, `bigdec`,
  `format "%04x"`, `NaN?`, `infinite?`, transients, `nth`/`count` on strings).
- **For the scoped arms:** a `cljg.io` host Reader/Writer (unblocks
  `read`/`write` streaming, `:extra-data-fn`, `print-json`, `write-json`),
  `clojure.pprint` (unblocks `pprint`), and host temporal/UUID types (unblocks
  those JSONWriter extensions).
- **Optionally** a host `utf16-escape`/`utf16-decode` helper (or a pure-Clojure
  surrogate layer) to close the astral-plane residual.

## Verification method / caveat

`cljgo` has no `load-file`, so verification inlined the satellite ahead of the
driver forms. The real integration must load it through the Go lib provider
(the loader path used by `clojure.string`/`clojure.core.async`). The
inline-vs-loader difference is only *how the ns is created*; the interned vars
and behavior are the same.
