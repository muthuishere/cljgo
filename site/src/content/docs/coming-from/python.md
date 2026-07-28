---
title: "Coming from Python"
sidebar:
  label: "From Python"
  order: 3
---

The Python instincts that transfer best are the ones you like most: dicts
and lists everywhere, functions passed around freely, a REPL you live in.

The two that need adjusting: collections don't mutate, and there is no
virtualenv — a cljgo project ships as one binary with nothing to install
on the other end.

## Start small: a dict is a map

Python:

```python
scores = {"suren": 91, "shaama": 78}
print(scores["suren"])
print(scores.get("mina", 0))
```

cljgo:

```clojure
(def scores {"suren" 91 "shaama" 78})

(println (get scores "suren"))
(println (get scores "mina" 0))
```

Output:

```
91
0
```

Same `get`, same default-if-missing. No commas needed between pairs —
whitespace is enough — though you may write them if you like; Clojure
treats a comma as blank space.

## But updating gives you a new map

Python mutates in place: `scores["mina"] = 84`. Here you get a new map
back and the old one is untouched:

```clojure
(def scores {"suren" 91 "shaama" 78})

(println (assoc scores "mina" 84))
(println scores)
```

Output:

```
{suren 91, shaama 78, mina 84}
{suren 91, shaama 78}
```

This is the single biggest change, and it's the one that pays. No more
"which function quietly modified my list?". A value you pass into a
function comes back the way you left it, always.

The two maps share their internals, so this is not the `copy.deepcopy()`
you're braced for — it's cheap.

## A list is a vector

```clojure
(def fruit ["apple" "mango" "fig"])

(println (count fruit))
(println (nth fruit 1))
(println (conj fruit "plum"))
```

Output:

```
3
mango
[apple mango fig plum]
```

`conj` adds an item and hands back the new vector. Indexing is `nth`.

## A comprehension is `for` — or just `map` and `filter`

Python:

```python
[n * n for n in range(1, 6)]
```

cljgo:

```clojure
(println (for [n (range 1 6)]
           (* n n)))
```

Output:

```
(1 4 9 16 25)
```

Nearly the same words in nearly the same order. For the filtered version,
the idiomatic move is a pipeline rather than a trailing `if`:

```clojure
(println (->> (range 1 11)
              (filter even?)
              (map #(* % %))))
```

Output:

```
(4 16 36 64 100)
```

`->>` threads the value down the chain, so you read it top to bottom.
`#(* % %)` is a lambda, with `%` as its argument — a shorter `lambda n:
n * n`.

Like Python's generators these are **lazy**: nothing is computed until
something asks.

## A decorator is a function that returns a function

Python:

```python
def with_logging(f):
    def wrapper(*args):
        print("calling with", args)
        result = f(*args)
        print("->", result)
        return result
    return wrapper
```

cljgo — the same idea, minus the `@` syntax:

```clojure
(defn with-logging [f]
  (fn [& args]
    (println "calling with" args)
    (let [result (apply f args)]
      (println "->" result)
      result)))

(def add (with-logging +))

(add 2 3)
```

Output:

```
calling with (2 3)
-> 5
```

`(fn [& args] ...)` is the inner wrapper; `&` collects varargs just like
`*args`. `apply` splices them back out when calling the wrapped function.
You wrap by hand — `(def add (with-logging +))` — which is what `@` was
doing anyway.

## Duck typing gets a name: protocols

Python trusts that anything with `.describe()` will do. cljgo lets you say
so explicitly, and — importantly — you can add the behaviour to types you
didn't write:

```clojure
(defprotocol Describe
  (describe [x]))

(extend-protocol Describe
  String
  (describe [x] (str "a string of " (count x) " characters"))

  Long
  (describe [x] (str "the number " x)))

(println (describe "hello"))
(println (describe 42))
```

Output:

```
a string of 5 characters
the number 42
```

That's monkey-patching without the monkey: the extension is scoped, typed
and doesn't edit anyone's class.

## Put it together: word frequencies

The classic `collections.Counter` job:

```clojure
(require '[clojure.string :as str])

(defn word-counts [text]
  (frequencies (str/split (str/lower-case text) #"\s+")))

(println (word-counts "the cat and the hat and the bat"))
```

Output:

```
{the 3, cat 1, and 2, hat 1, bat 1}
```

`#"\s+"` is a regex literal. `frequencies` is `Counter`, built in. One
line of real work.

## The REPL, and how it's different

Start it with `cljgo repl`:

```
user=> (+ 1 2)
3
user=> (map inc [1 2 3])
(2 3 4)
user=> (count "hello")
5
```

So far, `python3` with extra parentheses. The difference is what happens
when you connect it to a program that is already running: you can redefine
a function *inside the live process* and the next request uses it. No
restart, no reload hooks, no losing your state. Most Clojure work happens
this way — you leave the program running for hours and edit it underneath.

## Shipping: no virtualenv, no requirements.txt

Dependencies and build live in one file, `build.cljgo`:

```clojure
(defn build [b]
  (let [app (exe b {:name "newapp"
                    :main "src/newapp/core.cljg"})]
    (install b app)
    (run b app)))
```

```bash
cljgo run hello.clj              # like `python hello.py`
cljgo build -o hello hello.clj   # one static binary
./hello
```

Output:

```
Hello from a static binary!
```

Nothing to `pip install` on the server, no interpreter version to match,
no environment to activate. Copy the file, run it.

Ready for the language itself? Start at **1. Hello World** — it assumes
zero Clojure and moves in small steps.
