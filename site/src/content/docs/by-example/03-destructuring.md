---
title: "Destructuring"
sidebar:
  label: "3. Destructuring"
  order: 3
---

Destructuring means: instead of digging values out of a collection one by
one, you draw a little picture of its shape and cljgo fills in the names.

## Start small: take a pair apart

```clojure
(let [[a b] [10 20]]
  (println a b))
```

Output:

```
10 20
```

`let` binds names. `[a b]` on the left mirrors the shape of `[10 20]` on
the right — so `a` becomes `10` and `b` becomes `20`. No `first`, no
`second`, no indexing.

## First one, and the rest

```clojure
(let [[first-one & rest-ones] [1 2 3 4]]
  (println first-one)
  (println rest-ones))
```

Output:

```
1
(2 3 4)
```

`&` says "collect whatever is left". Same idea as varargs, but on the
binding side.

## Maps: pick keys by name

Most real data is a map. `:keys` pulls out exactly the ones you need:

```clojure
(let [{:keys [name city]} {:name "Sreyash" :city "Chennai"}]
  (println name city))
```

Output:

```
Sreyash Chennai
```

## Defaults for missing keys

What if a key isn't there? `:or` gives it a fallback instead of `nil`:

```clojure
(let [{:keys [name city] :or {city "unknown"}} {:name "Vidya"}]
  (println name city))
```

Output:

```
Vidya unknown
```

## Put it together: a function with friendly options

Destructuring works directly in a function's argument list — this is how
Clojure APIs take "options maps" without any boilerplate:

```clojure
(defn greet [{:keys [name city] :or {city "somewhere"}}]
  (str "Hello " name " from " city "!"))

(println (greet {:name "Sreyash" :city "Chennai"}))
(println (greet {:name "Vidya"}))
```

Output:

```
Hello Sreyash from Chennai!
Hello Vidya from somewhere!
```

One argument, self-documenting, forgiving about what's missing. You'll see
this shape everywhere in cljgo — `(http/serve {:port 8080 :handler app})`
is exactly this.
