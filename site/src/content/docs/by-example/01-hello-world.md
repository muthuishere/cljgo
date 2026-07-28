---
title: "Hello World"
sidebar:
  label: "1. Hello World"
  order: 1
---

Welcome to **cljgo by Example** — a numbered tour of Clojure and cljgo, one
runnable example per page. Every example on these pages is a real test in
cljgo's conformance suite: CI runs it in both the interpreter and a compiled
binary on every commit, so what you read is what runs.

No prior Clojure needed. If you're arriving from Go, Java, Python or C, the
*Coming from…* chapters map your instincts to the Clojure way.

Here is the traditional program:

```clojure
(println "Hello, world!")
```

Output:

```
Hello, world!
```

`println` is a function; `(...)` is a function call — the function comes
first, arguments after. That one rule is most of the syntax you'll ever learn.

Run it yourself — save as `hello.clj` and:

```bash
cljgo run hello.clj   # interpreted, instant
cljgo build           # or ship it as a ~6 MB static binary
```
