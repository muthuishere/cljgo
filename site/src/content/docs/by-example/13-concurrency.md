---
title: "Concurrency"
sidebar:
  label: "13. Concurrency"
  order: 13
---

Doing several things at once is normally where programs get scary: shared
memory, locks, races. cljgo runs on Go, so the cheap thing — the
**goroutine** — is right there, and Clojure gives you safe ways to share
between them.

## Start small: shared state is already safe

Page 4's atom was built for exactly this. Here a hundred concurrent
workers bump the same counter:

```clojure
(def hits (atom 0))

(def workers
  (doall
    (for [_ (range 100)]
      (future (swap! hits inc)))))

(doseq [w workers] @w)

(println @hits)
```

Output:

```
100
```

Exactly 100, every run. `swap!` is atomic, so no update is ever lost —
this is the classic race-condition bug, and it simply cannot happen here.

`(future body)` is the new part: it runs `body` on its own goroutine,
right now, and hands you a box. `@w` waits for that box to be filled.

## Futures: do the slow work in parallel

`future` is how you turn three sequential waits into one:

```clojure
(defn slow-sum [n]
  (reduce + (range n)))

(def a (future (slow-sum 3000000)))
(def b (future (slow-sum 3000000)))
(def c (future (slow-sum 3000000)))

(println (+ @a @b @c))
```

Output:

```
13499995500000
```

All three sums start immediately and run on different CPU cores; `@a`
blocks only until *that* one is done. Run it under `time` and you'll see
over 400% CPU — real parallelism, not an illusion.

## Channels: goroutines that talk

Sometimes you don't want a single answer at the end — you want a stream of
values moving between workers. That's a **channel**: a pipe you put values
into and take values out of.

```clojure
(require '[clojure.core.async :as a])

(def c (a/chan 1))

(a/go
  (a/>! c "hello from a goroutine"))

(println (a/<!! c))
```

Output:

```
hello from a goroutine
```

Three pieces:

- `(a/chan 1)` — a channel that can hold 1 value
- `(a/go ...)` — run this body on a goroutine
- `a/>!` puts a value in; `a/<!!` takes one out, waiting if it must

This is Clojure's `core.async`, and on cljgo it is not a simulation.
**Every `go` block is a real Go goroutine and every channel is a real Go
channel.** On the JVM, `go` is an elaborate compiler rewrite that fakes
lightweight threads; cljgo deletes that machinery because Go already has
the real thing.

A `go` block also *returns* a channel carrying its result, so you can read
a value straight back out of it:

```clojure
(require '[clojure.core.async :as a])

(println (a/<!! (a/go (+ 1 2))))
```

Output:

```
3
```

## Put it together: fan out, then collect

The everyday pattern: hand each piece of work to its own goroutine, let
them all run, and gather the answers off one channel.

```clojure
(require '[clojure.core.async :as a])
(require '[clojure.string :as str])

(defn word-count [page]
  (count (str/split page #"\s+")))

(def pages
  {"intro"   "welcome to the book"
   "setup"   "install cljgo and run it"
   "errors"  "try catch and ex info"
   "async"   "go blocks are goroutines"})

(def results (a/chan (count pages)))

(doseq [[title text] pages]
  (a/go
    (a/>! results [title (word-count text)])))

(def counts
  (into {}
        (for [_ (range (count pages))]
          (a/<!! results))))

(doseq [[title n] (sort counts)]
  (println title "->" n "words"))

(println "total:" (reduce + (vals counts)))
```

Output:

```
async -> 4 words
errors -> 5 words
intro -> 4 words
setup -> 5 words
total: 18
```

Read it as three beats. **Fan out:** one `go` block per page, all started
in a blink. **Collect:** take exactly as many results as you sent, in
whatever order they finish. **Combine:** `into {}` turns the `[title n]`
pairs into a map, and the rest is ordinary code again.

Swap `word-count` for an HTTP fetch or a database query and the shape
doesn't change — that's the point. Four pages here, four thousand would
work the same way, because a goroutine costs a couple of kilobytes.
