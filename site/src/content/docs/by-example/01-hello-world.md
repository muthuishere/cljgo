---
title: "Hello World"
sidebar:
  label: "1. Hello World"
  order: 1
---

Welcome to **cljgo by example** — a book that teaches you cljgo one small
step at a time. Every page starts tiny, grows a little, and ends with
something real. Every example is runnable exactly as shown.

No prior Clojure needed.

## Say hello

```clojure
(println "Hello, world!")
```

Output:

```
Hello, world!
```

That's a complete program. `println` is a function, and `(...)` calls it —
the function comes first, then its arguments. This one rule is most of the
syntax you will ever need.

## Print more than one thing

`println` takes any number of arguments and puts spaces between them:

```clojure
(println "1 + 2 is" (+ 1 2))
```

Output:

```
1 + 2 is 3
```

Notice `(+ 1 2)` — even `+` is just a function called the same way.

## Your first function

`defn` defines a function. This one takes a name and builds a greeting:

```clojure
(defn greet [who]
  (str "Hello, " who "!"))

(println (greet "cljgo"))
```

Output:

```
Hello, cljgo!
```

`str` glues values into a string. `greet` returns it — no `return` keyword
needed, a function always returns its last expression.

## Run it

Save any of these as `hello.clj`:

```bash
cljgo run hello.clj    # runs instantly, no build step
cljgo build            # or compile it into one small static binary
```

Next: the values you'll be greeting →
