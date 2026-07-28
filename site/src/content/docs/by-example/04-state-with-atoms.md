---
title: "State with atoms"
sidebar:
  label: "4. State with atoms"
  order: 4
---

Values in cljgo never change — which raises an obvious question: where does
*changing* state live? In an **atom**: one box, holding one value, changed
only by applying a function to it. Safe even with thousands of goroutines.

## Start small: a counter

```clojure
(def counter (atom 0))

(swap! counter inc)
(swap! counter inc)

(println @counter)
```

Output:

```
2
```

Three things happened:

- `(atom 0)` made the box, starting at `0`
- `swap!` changed it by applying a function — here `inc`, "add one"
- `@counter` read the current value out

You never assign into the box. You hand `swap!` a function, and the change
is atomic — no locks, no race.

## Grow a list

`swap!` can take extra arguments, passed along to the function:

```clojure
(def todo (atom []))

(swap! todo conj "buy milk")
(swap! todo conj "write docs")

(println @todo)
```

Output:

```
[buy milk write docs]
```

`(swap! todo conj "buy milk")` means: replace the value with
`(conj current-value "buy milk")`.

## Put it together: counting votes

```clojure
(def votes (atom {}))

(defn vote! [who]
  (swap! votes update who (fnil inc 0)))

(vote! "harsh")
(vote! "shaama")
(vote! "harsh")

(println @votes)
```

Output:

```
{harsh 2, shaama 1}
```

`update` bumps one key inside the map; `(fnil inc 0)` says "if this person
has no count yet, start from 0". Three votes, from anywhere in your
program, any goroutine — always consistent.
