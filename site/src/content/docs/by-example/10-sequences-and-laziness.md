---
title: "Sequences & laziness"
sidebar:
  label: "10. Sequences & laziness"
  order: 10
---

You almost never write a loop in cljgo. Instead you say what should happen
to each item, and three functions do the walking: `map`, `filter`,
`reduce`.

## Start small: change every item

```clojure
(println (map inc [1 2 3]))
```

Output:

```
(2 3 4)
```

`map` applies a function to each item and hands back the results. The
input vector is untouched — you got a new sequence.

## Keep some, combine the rest

`filter` keeps the items a test says yes to. `reduce` folds the whole
thing down to one value:

```clojure
(println (filter even? [1 2 3 4 5 6]))

(println (reduce + [1 2 3 4]))
```

Output:

```
(2 4 6)
10
```

`even?` is a **predicate** — a function that answers true or false. The
trailing `?` is just a naming convention, and you'll see it everywhere.

`reduce` starts with the first two items, adds them, then adds the next to
that running total, and so on: `1 + 2 + 3 + 4`.

## One vocabulary, every collection

Notice the answers above came back in parentheses even though the input
was a vector. That's a **sequence** — a plain "first item, then the rest"
view that every collection can produce. So the same three functions work
on all of them:

```clojure
(println (map inc [1 2 3]))

(println (map inc '(7 8 9)))

(println (map inc #{10 20 30}))

(println (map key {:a 1 :b 2}))
```

Output:

```
(2 3 4)
(8 9 10)
(21 31 11)
(:a :b)
```

Learn `map` once and you know it for vectors, lists, sets, maps and
strings. The set came back in its own order, as sets do.

## Sequences can be endless

`(range)` counts from zero and never stops. That would hang most
languages. Here it's fine, because a sequence only computes an item when
someone asks for one — that's **laziness**:

```clojure
(println (take 5 (range)))

(println (take 3 (map #(* % %) (range))))
```

Output:

```
(0 1 2 3 4)
(0 1 4)
```

The second line maps squaring over infinity and finishes instantly. `take`
asked for three items, so exactly three multiplications happened. You
describe the whole computation; only the part you consume actually runs.

## Reading left to right with `->>`

Chain a few of these and the parentheses nest backwards — the first step
ends up deepest inside. `->>` unwinds that. It takes each result and drops
it in as the **last** argument of the next line:

```clojure
(def prices [120 45 80 200])

(println
  (->> prices
       (filter #(> % 50))
       (reduce +)))
```

Output:

```
400
```

Read it top to bottom, like a recipe: take `prices`, keep the ones over
50, add them up. That's `(reduce + (filter #(> % 50) prices))` written the
way you actually think about it.

## Put it together: the three priciest items

```clojure
(def items
  [{:name "desk"   :price 8000}
   {:name "chair"  :price 3500}
   {:name "lamp"   :price 1200}
   {:name "laptop" :price 65000}
   {:name "mouse"  :price 900}
   {:name "screen" :price 18000}])

(println
  (->> items
       (filter #(> (:price %) 1000))
       (sort-by :price >)
       (take 3)
       (map :name)))
```

Output:

```
(laptop screen desk)
```

Five lines, five plain sentences: start with the items, drop anything
under 1000, sort by price descending, keep three, take their names. No
index variable, no accumulator, no mutation — and every step is a function
you could have written yourself.

Next: what happens when the answer depends on a condition →
