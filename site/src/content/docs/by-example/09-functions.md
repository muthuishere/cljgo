---
title: "Functions"
sidebar:
  label: "9. Functions"
  order: 9
---

Functions are the only unit of code in cljgo. There are no classes and no
methods to hang them off — you write a function, and you pass it around
like any other value.

## Start small: name a function

```clojure
(defn double-it [x]
  (* 2 x))

(println (double-it 21))
```

Output:

```
42
```

`defn` takes a name, a vector of parameters, and a body. The body's last
expression is the return value — there is no `return`.

## One function, several arities

A function can accept different numbers of arguments, each with its own
body. Wrap each version in its own parentheses:

```clojure
(defn greet
  ([who]
   (greet "Hello" who))
  ([greeting who]
   (str greeting ", " who "!")))

(println (greet "Asha"))
(println (greet "Good morning" "Ravi"))
```

Output:

```
Hello, Asha!
Good morning, Ravi!
```

The one-argument version just calls the two-argument one with a default.
That's the usual pattern: the short arity supplies defaults, the long
arity does the work. No overloading rules to learn, no `null` checks.

## Functions without names

Sometimes a function is too small to deserve a name. `fn` makes one on the
spot:

```clojure
(println (map (fn [x] (* x x)) [1 2 3 4]))

(println (map #(* % %) [1 2 3 4]))
```

Output:

```
(1 4 9 16)
(1 4 9 16)
```

Those two lines are the same function written twice. `#(...)` is shorthand
for `fn`, and `%` stands for its argument. Use `#()` for one-liners; the
moment it needs a second line, switch to `fn` so the argument gets a real
name.

## A function is just a value

Nothing special happens when you pass a function around. It goes in a
`def`, in a vector, in a map — anywhere a number could go:

```clojure
(defn shout [s]
  (str s "!"))

(def loud shout)

(println (loud "again"))

(def ops [inc dec])

(println ((first ops) 10))
```

Output:

```
again!
11
```

`inc` and `dec` are ordinary functions from the standard library, sitting
in a vector. `(first ops)` pulls one out and the extra pair of parentheses
calls it.

## Put it together: a report that takes its own formatter

Here's why this matters. Write the *shape* of the job once, and let the
caller supply the detail as a function:

```clojure
(require '[clojure.string :as str])

(def prices [120 45 80])

(defn report [format-one items]
  (str/join "\n" (map format-one items)))

(defn plain [n]
  (str "Rs " n))

(defn fancy [n]
  (str "** Rs " n " **"))

(println (report plain prices))
(println "---")
(println (report fancy prices))
```

Output:

```
Rs 120
Rs 45
Rs 80
---
** Rs 120 **
** Rs 45 **
** Rs 80 **
```

`report` knows nothing about rupees, stars, or currency. It knows "apply
the given function to each item and join the results". A new format is a
new three-line function, never a change to `report`.

This is the plug-in point that other languages need an interface, a
subclass, or a strategy object for. Here it's an argument.

Next: the functions that walk collections for you — `map`, `filter`,
`reduce` →
