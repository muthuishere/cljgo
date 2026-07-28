---
title: "Collections"
sidebar:
  label: "8. Collections"
  order: 8
---

Most programs are mostly data. cljgo gives you four containers for it, each
with its own literal syntax, and they all answer the same handful of
questions.

## Start small: a vector

A **vector** is an ordered list of things. Square brackets, no commas
needed:

```clojure
(def fruits ["mango" "apple" "fig"])

(println (count fruits))
(println (nth fruits 0))
```

Output:

```
3
mango
```

Use a vector when order matters and you want to look things up by
position. It's the workhorse — when in doubt, reach for a vector.

## Maps: look things up by name

A **map** pairs keys with values. Curly braces, key then value:

```clojure
(def book {:title "Dune" :author "Herbert" :year 1965})

(println (:title book))
(println (:pages book))
(println (get book :pages "unknown"))
```

Output:

```
Dune
nil
unknown
```

Those `:title`-style names are **keywords** — cheap, self-naming labels.
A keyword doubles as a function that looks itself up, which is why
`(:title book)` reads so nicely.

Missing keys give you `nil` rather than an error. If you'd rather have a
fallback, `get` takes one as a third argument.

## Sets: membership, no duplicates

A **set** answers exactly one question well: is this thing in here?

```clojure
(def tags #{"go" "clojure"})

(println (contains? tags "go"))
(println (contains? tags "rust"))

(println (conj tags "go"))
```

Output:

```
true
false
#{clojure go}
```

Adding `"go"` a second time changed nothing — a set holds each value once.
Notice the printed order isn't the order you typed: a set has no order, so
never rely on it.

## Lists: what code is made of

A **list** is written with parentheses. That's the same syntax as a
function call, so you quote it to mean "data, don't run this":

```clojure
(def numbers '(1 2 3))

(println numbers)
(println (first numbers))
```

Output:

```
(1 2 3)
1
```

That leading `'` is the quote. Without it, cljgo would try to call `1` as
a function.

You'll write lists rarely and read them constantly — every piece of cljgo
code you've seen so far *is* a list. Reach for a vector for your own data.

## The same words work everywhere

Four containers, one vocabulary. `count` counts, `conj` adds:

```clojure
(println (conj [1 2] 3))
(println (conj '(1 2) 3))
(println (conj #{1 2} 3))
(println (conj {:a 1} [:b 2]))

(println (count [1 2 3]))
(println (count {:a 1 :b 2}))
(println (count #{1 2 3}))
```

Output:

```
[1 2 3]
(3 1 2)
#{1 3 2}
{:a 1, :b 2}
3
2
3
```

Look closely at the lists: `conj` put `3` at the *front*. That's not a
quirk — `conj` always adds wherever the collection is fastest, the end for
a vector, the front for a list. You get the cheap operation by default.

And nothing was modified. `(conj [1 2] 3)` returned a *new* vector; the
original `[1 2]` is untouched. Every collection here works that way.

## Put it together: a little library

Real data is usually maps inside a vector — a table, basically. Here are
three books:

```clojure
(def library
  [{:title "Dune"        :author "Herbert" :year 1965 :read? true}
   {:title "Neuromancer" :author "Gibson"  :year 1984 :read? false}
   {:title "Piranesi"    :author "Clarke"  :year 2020 :read? true}])

(println (count library))

(println (:title (first library)))

(println (map :author library))

(defn describe [book]
  (str (:title book) " by " (:author book) " (" (:year book) ")"))

(println (describe (last library)))
```

Output:

```
3
Dune
(Herbert Gibson Clarke)
Piranesi by Clarke (2020)
```

No class to declare, no schema to migrate, no getters. A record is a map;
a table is a vector of them; `(map :author library)` pulls one column out.

Look at `(map :author library)` once more: its first argument is a
*function*. Passing functions around is the next idea →
