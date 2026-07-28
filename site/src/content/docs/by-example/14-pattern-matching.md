---
title: "Pattern matching"
sidebar:
  label: "14. Pattern matching"
  order: 14
---

Destructuring (page 3) pulls a value apart when you already know its
shape. **Pattern matching** goes further: you list several shapes, and the
first one that fits wins. It's a chain of `if`s that reads like a table.

## Start small: match on literal values

```clojure
(require '[clojure.core.match :refer [match]])

(defn describe [n]
  (match [n]
    [0] "zero"
    [1] "one"
    :else "many"))

(println (describe 0))
(println (describe 1))
(println (describe 7))
```

Output:

```
zero
one
many
```

Read `match` as pairs: **pattern, then result**. The thing being matched
goes in a vector — `[n]` — and so does each pattern — `[0]`, `[1]`.
`:else` is the fallback when nothing matched.

Why the vectors? Because `match` can test several values at once, and you
are about to.

## Two values at a time

```clojure
(require '[clojure.core.match :refer [match]])

(defn move [[x y]]
  (match [x y]
    [0 0] "at the origin"
    [x 0] (str "on the x axis at " x)
    [0 y] (str "on the y axis at " y)
    [x y] (str "somewhere at " x "," y)))

(println (move [0 0]))
(println (move [5 0]))
(println (move [0 3]))
(println (move [2 7]))
```

Output:

```
at the origin
on the x axis at 5
on the y axis at 3
somewhere at 2,7
```

Now the payoff is visible. A literal like `0` must be equal; a plain name
like `x` matches anything **and binds it** — you can use `x` on the right
of that row. Rows are tried top to bottom, so the specific case sits above
the general one.

## Guards: patterns with a condition

Sometimes "equal to" isn't enough. `(x :guard pred)` matches when `pred`
says yes:

```clojure
(require '[clojure.core.match :refer [match]])

(defn classify [n]
  (match [n]
    [(x :guard neg?)]        "negative"
    [0]                      "zero"
    [(x :guard #(< % 10))]   "small"
    :else                    "big"))

(println (classify -4))
(println (classify 0))
(println (classify 3))
(println (classify 900))
```

Output:

```
negative
zero
small
big
```

## Matching inside maps

Maps work too — name a key, give it a pattern:

```clojure
(require '[clojure.core.match :refer [match]])

(defn describe [order]
  (match [order]
    [{:status :paid   :total t}] (str "paid, " t " rupees")
    [{:status :unpaid :total t}] (str "still owes " t " rupees")
    [{:status s       :total _}] (str "status is " s)))

(println (describe {:status :paid :total 250}))
(println (describe {:status :unpaid :total 90}))
(println (describe {:status :cancelled :total 0}))
```

Output:

```
paid, 250 rupees
still owes 90 rupees
status is :cancelled
```

`:status :paid` demands that exact value; `:total t` binds whatever is
there; `_` means "anything, don't care". Keep the same set of keys across
the rows of one `match` — that's the shape that behaves predictably today.

## Put it together: a tiny command dispatcher

The classic use: a verb and its argument, matched as a pair.

```clojure
(require '[clojure.core.match :refer [match]])
(require '[clojure.string :as str])

(def cart (atom []))

(defn run-command [verb arg]
  (match [verb arg]
    ["add"    item]          (do (swap! cart conj item)
                                 (str "added " item))
    ["remove" item]          (do (swap! cart #(vec (remove #{item} %)))
                                 (str "removed " item))
    ["list"   _]             (str "cart: " (str/join ", " @cart))
    ["count"  _]             (str (count @cart) " item(s)")
    [(v :guard string?) _]   (str "I don't know how to " v)))

(defn dispatch [line]
  (let [[verb arg] (str/split line #" " 2)]
    (println (run-command verb arg))))

(dispatch "add mangoes")
(dispatch "add rice")
(dispatch "list")
(dispatch "remove rice")
(dispatch "count")
(dispatch "yodel loudly")
```

Output:

```
added mangoes
added rice
cart: mangoes, rice
removed rice
1 item(s)
I don't know how to yodel
```

Every command is one line, and the whole command language is readable in
one glance. Adding a verb means adding a row.

This isn't a slow chain of tests, either. `match` is a *macro*: at compile
time it studies your rows and builds a decision tree — the Maranget
algorithm — that tests each part of the input as few times as possible. By
the time your program runs, the table is gone and only fast branching is
left.
