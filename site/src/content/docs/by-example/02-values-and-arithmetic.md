---
title: "Values & arithmetic"
sidebar:
  label: "2. Values & arithmetic"
  order: 2
---

Numbers work the way you expect — with one twist that makes them nicer.

## Start small

```clojure
(+ 1 2)
```

Output:

```
3
```

`+` is a function, so it goes first. The bonus: it takes *any* number of
arguments.

```clojure
(* 2 3 4)
```

Output:

```
24
```

## Comparisons chain

In most languages you write `1 < 2 && 2 < 3`. Here you just list the
numbers:

```clojure
(< 1 2 3)
```

Output:

```
true
```

`(< 1 2 3)` asks "is this sequence increasing?" — three numbers, one call.

## Division that doesn't lie

Integer division and remainder are separate, honest functions:

```clojure
(quot 17 5)
(rem 17 5)
```

Output:

```
3
2
```

## Put it together: average a shopping list

```clojure
(def prices [120 45 80])

(reduce + prices)
```

Output:

```
245
```

`reduce` feeds the vector through `+` one element at a time —
`120 + 45 + 80`.

```clojure
(/ (reduce + prices) (count prices))
```

Output:

```
245/3
```

That `245/3` is a **ratio** — cljgo keeps the exact fraction instead of
silently rounding. Ask for a decimal only when you actually want one:

```clojure
(double (/ (reduce + prices) (count prices)))
```

Output:

```
81.66666666666667
```

Exactness by default, rounding by choice.
