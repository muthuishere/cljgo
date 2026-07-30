# ADR 0110 — bytes, dates, base64, and three `clojure.core` parity bugs

Date: 2026-07-30 · Status: **accepted — all five asks implemented and conformance-frozen 2026-07-30** (byte I/O `cljg.io`/`cljg.stream`, `cljg.date` ISO format/parse, `cljg.security` base64, the bare-`str` UUID split, and the fn arity of `clojure.string/replace`/`replace-first`). Two verifier-found defects closed on top: the anchored/zero-width regex walk, and byte-route sign parity so producers and consumers compose.

Follows **ADR 0109** (`require-go` reaches the Go stdlib), which closed the four
blockers koine was waiting on. This ADR is the *next* koine-side statement of
need, raised the same way: measured against the installed cljgo, each item with
the exact probe that produced it, and each saying what unblocks it.

Touches `cljg.io` / `cljg.stream` (**ADR 0101**'s stdlib-gap program),
`cljg.date`, and `clojure.core` parity (**ADR 0046**'s link-set discipline
applies — every ask below is referenced-symbol-only, so a program that never
calls it never links it).

## Context

koine 0.1.0 shipped on 2026-07-30 with cljgo as a tier-1 host: all nine
namespaces pass conformance on JVM **and** cljgo, interpreted and as AOT
binaries, including a real MCP stdio handshake through `koine.process/spawn`.
None of koine's cljgo branches uses `require-go` — they go through `cljg.system`,
`cljg.process`, `cljg.date`, `cljg.net.http`, which is what makes them behave
identically under `cljgo run` and `cljgo build`.

So this is not a complaint about a broken host. It is the list of things that
stop koine from covering the *rest* of what a library author needs, ranked by how
much they block.

Every measurement below is from `cljgo run` on the installed binary
(0.1.0-dev, Go 1.26.3), 2026-07-30, with the JVM as the reference oracle.

---

## The asks, ranked

### 1. Byte-level I/O — there is no route between a file and bytes

**blocker** for a binary `koine.fs` seam, for reading any non-UTF-8 file, and for
MCP resource/blob content.

Byte arrays themselves work fine — `(byte-array 3)` and `alength` are present,
`bytes?` is true of what `cljg.compress/gzip` returns, and `gunzip` round-trips
to `[104 101 108 108 111]`. The *primitive* is there. What is missing is every
door into it:

```clojure
(resolve 'cljg.io/read-bytes)    ;=> nil   ; also write-bytes, slurp-bytes, spit-bytes
(resolve 'cljg.io/reader)        ;=> nil   ; also writer, input-stream
(resolve 'cljg.stream/of-file)   ;=> nil   ; also file-reader, file-writer
(.getBytes "ab")                 ;=> "no method getBytes on string"
```

`cljg.stream` handles exist and are good, but they can only be *obtained* from a
process pipe (`cljg.process/spawn`) or an HTTP body (`:as :stream`) — never from
a path. And `slurp` is not a workaround: a 12-byte binary file reads back as 13
characters (lossy replacement runes). That is identically lossy on the JVM, so it
is not a divergence — it just means neither host offers a text route to bytes,
and cljgo lacks the byte one.

**Ask:** `cljg.io/read-bytes` + `write-bytes` on a path (ADR 0101 already lists
these as the only genuinely missing `cljg.io` items), and ideally
`cljg.stream/of-file` so the ONE stream abstraction covers files too, not just
pipes and sockets.

### 2. `(str (random-uuid))` returns the reader tag

**major** — silent wire corruption, and the cheapest fix here.

```clojure
;; cljgo
(str (random-uuid))  ;=> "#uuid \"2a9e77d6-adc3-4d5a-8dae-8d04a9e5b13d\""   (44 chars)
;; JVM oracle
(str (random-uuid))  ;=> "2a9e77d6-adc3-4d5a-8dae-8d04a9e5b13d"             (36 chars)
```

This one is nasty precisely because it does not throw. Any id that goes on a wire
— a JSON-RPC `id`, an MCP session id, a correlation id in a log line — silently
gains `#uuid "` and a trailing quote. The peer sees a malformed id and the bug
surfaces far from its cause.

**Ask:** `print-method` / `str` of a UUID prints the bare 36-char form, matching
the JVM. `pr-str` should keep the `#uuid` tag (that is correct and readable);
only `str` / `print` are wrong.

### 3. `clojure.string/replace` rejects a function replacement

**major** — a documented `clojure.core` arity that silently is not there.

```clojure
(str/replace "a1" #"\d" (fn [m] "X"))
;; cljgo => THROW: replace expects a string, got: #object[fn]
;; JVM   => "aX"
```

koine hit this for real: `koine.env/expand` used it, worked on the JVM, and threw
on cljgo the first time the check ran. It is now hand-rolled as a character scan
— which is fine for koine, but every library author who writes the idiomatic
thing will hit the same wall.

**Ask:** the fn arity of `clojure.string/replace` (and the same for
`replace-first`), receiving the match — a string for a group-less pattern, a
vector of groups otherwise, per the JVM contract.

### 4. Date formatting and parsing

**major** for anything a human or a protocol reads.

`cljg.date` is `nano-time` / `now` / `since` / `since-ms` — a stopwatch and an
epoch-millis clock, nothing else:

```clojure
(resolve 'cljg.date/format)   ;=> nil   ; also parse, format-iso, instant
```

That is enough for koine.time (durations and timestamps) and not enough for a
library that has to emit or accept an ISO-8601 instant, which is what every
protocol on the wire actually carries.

**Ask:** `cljg.date/format-iso` + `parse-iso` at minimum (RFC 3339 / ISO-8601
UTC), and `format` / `parse` with a pattern if the layout translation to Go's
reference-time format is cheap. ISO alone closes the protocol case.

### 5. base64

**minor** for koine today, **blocker** for MCP image/blob content later.

```clojure
(resolve 'cljg.security/base64-encode)  ;=> nil   ; sha256 IS present
(resolve 'clojure.core/base64-encode)   ;=> nil
```

`cljg.security` already has `sha256`, so the home for this is obvious.

**Ask:** `base64-encode` / `base64-decode` in `cljg.security`, over bytes and
strings. Pairs with ask 1 — base64 without a byte route only solves half.

---

## Explicitly NOT asks (retracted on measurement)

Two things were carried as cljgo divergences and are not:

- **The numeric tower is fine.** `(* 99999999999999 99999999999999)` throws on
  *both* hosts — "long overflow" on the JVM, "integer overflow" on cljgo — and
  `*'` promotes to `9999999999999800000000000001N` on both. cljgo matches
  Clojure here; the earlier "cljgo raises overflow where the JVM auto-promotes"
  claim was wrong, because plain `*` does not auto-promote on the JVM either.
- **`format` is correct.** `(format "%05.2f|%s" 3.14159 :x)` gives `"03.14|:x"`
  on both.

Also confirmed present and working on cljgo, so no seam is needed for any of
them: regex (`re-find` / `re-seq` / `split`), `deftype` / `defrecord` / `reify`
/ `defprotocol`, `future` / `promise` / `atom` / `agent` / `ref` + `dosync`,
transducers, `ex-info` / `ex-data`, `read-string`, `bigint`, ratios, core.async,
`sort-by` with a comparator, `byte-array` / `aget` / `alength`.

## Consequences

Asks 2 and 3 are parity bugs in code that already exists — small, and they
remove two silent-wrong-answer traps. Asks 1, 4, 5 are new surface, all
referenced-symbol-only, so ADR 0023's binary-size mandate is unaffected for
programs that do not call them.

With 1 and 5 in, koine can add a binary `fs` seam and MCP blob content; with 4,
a date seam. Until then koine documents them as known gaps rather than shipping
a JVM-only branch — a seam that throws on a tier-1 host is worse than no seam.

Separately, **ADR 0095** (Clojars consume + deploy) remains the one distribution
gap: `dep` takes `{:git …}` / `{:path …}` only, so a library published to Clojars
needs a second git coordinate for cljgo users. koine 0.1.0 ships both.
