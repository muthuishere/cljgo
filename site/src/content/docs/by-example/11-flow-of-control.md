---
title: "Flow of control"
sidebar:
  label: "11. Flow of control"
  order: 11
---

Choosing between two answers is the oldest idea in programming. The twist
in cljgo is that a choice is an **expression** — it produces a value, so
you can hand it straight to something else.

## Start small: `if`

```clojure
(println (if (> 3 2) "bigger" "smaller"))

(def label (if (> 3 2) "bigger" "smaller"))

(println label)
```

Output:

```
bigger
bigger
```

`if` takes three things: a test, the answer when it's true, the answer
when it's false. It doesn't *do* something to a variable — it *is* the
value, which is why the second line can `def` it directly.

## Only `nil` and `false` are falsey

This one surprises everybody, so learn it now. Zero is true. An empty
string is true. An empty vector is true:

```clojure
(println (if 0 "truthy" "falsey"))
(println (if "" "truthy" "falsey"))
(println (if [] "truthy" "falsey"))

(println (if nil "truthy" "falsey"))
(println (if false "truthy" "falsey"))
```

Output:

```
truthy
truthy
truthy
falsey
falsey
```

There are exactly two falsey values in the whole language: `nil` and
`false`. Everything else is true.

That rule is why `nil` is so useful — a missing map key comes back `nil`,
and you can test it directly. But if you want "is this collection empty?",
ask for it: `(seq coll)` gives `nil` for an empty collection, which is the
idiomatic emptiness test.

## `when`: one branch, several steps

If there's nothing to do in the false case, use `when`. It skips the
else-branch and lets you write more than one expression in the body:

```clojure
(def cart ["milk" "bread"])

(when (seq cart)
  (println "You have" (count cart) "items")
  (println "First one:" (first cart)))

(println (when (seq []) "never printed"))
```

Output:

```
You have 2 items
First one: milk
nil
```

The last line shows what `when` gives back when the test fails: `nil`.
Still an expression, still a value.

## `case`: match an exact value

When you're comparing one thing against a list of known constants, `case`
says it plainly:

```clojure
(defn day-kind [day]
  (case day
    :sat "weekend"
    :sun "weekend"
    "weekday"))

(println (day-kind :sat))
(println (day-kind :wed))
```

Output:

```
weekend
weekday
```

Pairs of value-and-result, then a bare final expression as the default.
`case` compares for equality only — no ranges, no tests. That makes it
fast, and it makes the intent obvious.

## `cond`: a list of tests

When the branches are *tests* rather than fixed values, use `cond`. It
walks the pairs top to bottom and takes the first one that's true:

```clojure
(defn grade [score]
  (cond
    (>= score 90) "A"
    (>= score 75) "B"
    (>= score 50) "C"
    :else         "F"))

(println (grade 95))
(println (grade 80))
(println (grade 51))
(println (grade 12))
```

Output:

```
A
B
C
F
```

`(grade 95)` matches the first test and stops — the later ones never run.
The final `:else` is not a keyword the language knows about; it's just a
keyword, and keywords are truthy, so it always matches. Any truthy value
would do, but everyone writes `:else`.

## Put it together: what to charge for shipping

```clojure
(defn shipping [order]
  (let [total (:total order)]
    (cond
      (:express? order) 250
      (>= total 2000)   0
      (>= total 500)    60
      :else             99)))

(println (shipping {:total 3000}))
(println (shipping {:total 800}))
(println (shipping {:total 120}))
(println (shipping {:total 3000 :express? true}))
```

Output:

```
0
60
99
250
```

Read the rules as a business would state them: express always costs 250;
otherwise big orders ship free, medium orders pay 60, small ones pay 99.
Order matters — `:express?` sits first because it wins over everything
else.

And look at the last call. `(:express? order)` is `nil` for the first
three orders, which is falsey, so that branch quietly fell through. No
`if key exists`, no default map, no exception.

Pair this with destructuring from page 3 and a whole class of
option-handling code disappears: name the keys you need in the argument
list, let `cond` decide, return a value.
