---
title: "Compression"
sidebar:
  label: "5. Compression"
  order: 5
---

gzip, deflate and zlib are built into cljgo — no library to install, and
they work the same in the REPL and inside a compiled 6 MB binary.

## Start small: shrink a string, get it back

```clojure
(require '[cljg.compress :as z])

(def packed (z/gzip "hello, hello, hello, hello"))

(z/gunzip packed {:as :string})
```

Output:

```
hello, hello, hello, hello
```

`gzip` returns compressed bytes; `gunzip` reverses it. `{:as :string}`
says "give me text back, not bytes".

## See it actually working

Repetitive data compresses dramatically. Watch the byte counts:

```clojure
(def report (apply str (repeat 200 "quarterly numbers ")))

(count report)
(count (z/gzip report))
```

Output:

```
3600
67
```

3600 characters in, **67 bytes** out — because gzip notices the
repetition. This is why compressed HTTP responses and log archives are
small.

## The other codecs

Same shape, different algorithm — `deflate`/`inflate` and
`zlib-compress`/`zlib-decompress`:

```clojure
(= report (z/inflate (z/deflate report) {:as :string}))
```

Output:

```
true
```

## Which do I pick?

- **gzip** — files and HTTP; the one you usually want
- **deflate** — the raw algorithm, smallest framing
- **zlib** — when another system says "zlib format"

All three round-trip anything you put in. For big inputs there are
streaming versions (`gunzip-stream`, …) that decompress as you read —
they appear again in the chapter on streams.
