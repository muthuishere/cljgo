---
title: "Coming from Go"
sidebar:
  label: "From Go"
  order: 1
---

You already know most of the runtime. cljgo compiles to plain Go source and
then builds it with the Go toolchain, so what comes out is a single static
binary that starts in milliseconds, needs no interpreter installed, and
cross-compiles the way you're used to.

What changes is the *shape of the code*: no structs, no methods, no
`err != nil` ladder. This page walks the four or five habits you'll want to
swap, smallest first.

## Start small: a struct is a map

Go:

```go
type User struct {
    Name string
    Age  int
}

u := User{Name: "Asha", Age: 30}
fmt.Println(u.Name)
```

cljgo:

```clojure
(def user {:name "Asha" :age 30})

(println (:name user))
(println (:age user))
```

Output:

```
Asha
30
```

`{...}` is a map. `:name` is a **keyword** — a self-naming constant that is
also a function, so `(:name user)` reads the key. No type declaration, no
tags, no `reflect` to inspect it later.

## Values don't change — you make new ones

This is the one real mental shift. In Go you'd write `u.Age = 31`. Here you
build a new map:

```clojure
(def user {:name "Asha" :age 30})

(def older (assoc user :age 31))

(println older)
(println user)
```

Output:

```
{:name Asha, :age 31}
{:name Asha, :age 30}
```

`assoc` returned a *new* map; the original is untouched. That is not a copy
in the expensive sense — the two maps share their structure internally.

Why bother? Because the value you handed to another goroutine can never be
mutated under you. Half the reason you reach for a mutex in Go disappears.

## A method is just a function

Go:

```go
func (u User) FullName() string {
    return u.First + " " + u.Last
}
```

cljgo — the receiver becomes an ordinary first argument:

```clojure
(defn full-name [u]
  (str (:first u) " " (:last u)))

(println (full-name {:first "Asha" :last "Iyer"}))
```

Output:

```
Asha Iyer
```

No method set, no pointer-vs-value receiver question. Functions are free
floating and take data.

## An interface is a protocol

When you *do* want one name to behave differently per type, that's a
**protocol** — Go's interface, with the dispatch written explicitly:

```clojure
(defprotocol Shape
  (area [s]))

(defrecord Rect [w h]
  Shape
  (area [s] (* (:w s) (:h s))))

(defrecord Circle [r]
  Shape
  (area [s] (* 3.14159 (:r s) (:r s))))

(println (area (->Rect 3 4)))
(println (area (->Circle 2)))
```

Output:

```
12
12.56636
```

`defrecord` makes a named map type; `->Rect` is its constructor. The
difference from Go: a protocol can be extended to a type you don't own,
after the fact — no wrapper type needed.

## Goroutines and channels are still goroutines and channels

`go` blocks are not a simulation. A `go` block runs on a real goroutine and
`chan` is a real Go channel underneath:

```clojure
(require '[clojure.core.async :as a])

(def results (a/chan 10))

(doseq [n [1 2 3 4]]
  (a/go (a/>! results (* n n))))

(def collected
  (sort (repeatedly 4 #(a/<!! results))))

(println collected)
```

Output:

```
(1 4 9 16)
```

Read the operators as arrows: `>!` puts onto a channel, `<!!` takes from
one. The `!!` version blocks the calling thread — that's why we can use it
at the top level, outside a `go` block.

We sorted the results because, exactly like Go, four goroutines finish in
whatever order they please.

## `err != nil`, two ways

The direct translation is **errors as values** — return a map that says
which happened:

```clojure
(defn parse-port [s]
  (if-let [n (parse-long s)]
    {:ok n}
    {:error (str "not a number: " s)}))

(defn report [s]
  (let [{:keys [ok error]} (parse-port s)]
    (if error
      (println "skipping —" error)
      (println "listening on" ok))))

(report "8080")
(report "eighty")
(report "9090")
```

Output:

```
listening on 8080
skipping — not a number: eighty
listening on 9090
```

That `{:keys [ok error]}` is destructuring — it pulls both keys out in one
line, the same way `a, err :=` does.

For the *exceptional* case — the one that should abort — use `ex-info`,
which is an error that carries a data map instead of a formatted string:

```clojure
(defn parse-port! [s]
  (if-let [n (parse-long s)]
    n
    (throw (ex-info "bad port" {:input s}))))

(println (parse-port! "8080"))

(try
  (parse-port! "eighty")
  (catch Exception e
    (println (ex-message e))
    (println (ex-data e))))
```

Output:

```
8080
bad port
{:input eighty}
```

`ex-data` gives you back the map, machine-readable. Use values for expected
failures, `ex-info` for the ones that mean "stop".

## Put it together: `go build` → `cljgo build`

A single file compiles straight to a binary:

```bash
cljgo run hello.clj              # like `go run`
cljgo build -o hello hello.clj   # like `go build`
./hello
```

Output:

```
Hello from a static binary!
```

That binary was 6.7 MB on this machine, links no interpreter, and runs
anywhere the Go toolchain can target — `cljgo dist` cross-compiles for
every platform in one go.

For a project, the build plan is `build.cljgo` — your `go.mod` plus your
Makefile, written in the same language as the app:

```clojure
(defn build [b]
  (let [app (exe b {:name "newapp"
                    :main "src/newapp/core.cljg"})]
    (install b app)
    (run b app)))
```

And a third-party Go module is one line inside it, not a separate file:

```clojure
(go-require app "github.com/gorilla/websocket" "v1.5.3")
```

Same toolchain, same deployment story, less ceremony in between.

Ready for the language itself? Start at **1. Hello World** and read
straight through — it assumes nothing.
