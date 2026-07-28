---
title: "Coming from C"
sidebar:
  label: "From C"
  order: 4
---

The familiar part first: cljgo produces a **single static binary**. No
runtime to install on the target machine, no shared libraries to chase, no
`LD_LIBRARY_PATH`. You compile, you copy the file, it runs.

Almost everything else is a simplification, and this page is mostly a list
of things you no longer have to do.

## Start small: no `malloc`, no `free`

There is a garbage collector — Go's — and it is not optional. You never
size a buffer, never own a pointer, never write a destructor. That removes
an entire category of bug from your day, and it costs you the ability to
control exactly when memory is released.

That trade is the whole deal. If it's acceptable, everything below follows.

## A struct is a map

C:

```c
struct Point { int x; int y; };

struct Point p = { .x = 3, .y = 4 };
printf("%d\n", p.x);
```

cljgo:

```clojure
(def p {:x 3 :y 4})

(println (:x p))
```

Output:

```
3
```

No declaration, no layout, no header. `{...}` is a map; `:x` is a keyword
that also works as the accessor function.

## Mutation becomes a new value

In C you'd write `p.x = 10;` — or pass `&p` so a function could. Here:

```clojure
(def p {:x 3 :y 4})

(def moved (assoc p :x 10))

(println moved)
(println p)
```

Output:

```
{:x 10, :y 4}
{:x 3, :y 4}
```

`assoc` built a new map. The original still says `3`.

This sounds wasteful and isn't: the two maps share their internal
structure, so you are not memcpy-ing the world on every update. What you
gain is that no function can quietly scribble through a pointer you handed
it — aliasing bugs stop existing.

## When you genuinely need mutable state: an atom

C's `static int counter;` has a direct equivalent. It's a box you change by
applying a function to it, and it is safe across threads without a mutex:

```clojure
(def total (atom 0))

(defn record! [reading]
  (swap! total + reading))

(record! 12)
(record! 8)
(record! 5)

(println @total)
```

Output:

```
25
```

`swap!` applies `+` to the current value and the new reading, atomically.
`@total` reads it out. No lock, no `volatile`, no memory-order question.

Note the shape: mutable state is *explicit and rare*, sitting in a named
box, instead of being the default property of every variable.

## Headers and translation units become one namespace

C splits every module in two — a `.h` that declares and a `.c` that
defines — and you keep them in sync by hand. Here a file declares its own
name and what it needs, and that's the whole story:

```clojure
(ns myapp.geometry
  (:require [clojure.string :as str]))

(defn area [{:keys [w h]}]
  (* w h))

(println (str/upper-case "area:") (area {:w 3 :h 4}))
```

Output:

```
AREA: 12
```

`ns` names this unit. `:require` pulls in another one under a short alias.
There is no forward declaration, no include guard, no link order — the
compiler resolves names from the namespace graph.

`{:keys [w h]}` in the argument list is destructuring: it takes a map and
binds `w` and `h` from it, the way you'd unpack a passed-in struct.

## Put it together: summarise some readings

The C version of this involves a fixed-size array, a length parameter, and
a loop with an accumulator. Here the data is a vector of maps and the work
is three expressions:

```clojure
(def readings
  [{:sensor "north" :temp 21}
   {:sensor "south" :temp 27}
   {:sensor "north" :temp 23}])

(defn summary [rs]
  {:count (count rs)
   :max   (apply max (map :temp rs))
   :mean  (double (/ (reduce + (map :temp rs))
                     (count rs)))})

(println (summary readings))
```

Output:

```
{:count 3, :max 27, :mean 23.666666666666668}
```

`(map :temp rs)` pulls the temperature out of every reading — the keyword
is the function. `reduce +` sums them. `apply max` hands the whole list to
`max` as separate arguments. Nothing was allocated by you, nothing was
freed by you, and no length was passed anywhere.

## `make` becomes `cljgo build`

```bash
cljgo run hello.clj              # run it straight away, no build
cljgo build -o hello hello.clj   # produce the binary
./hello
```

Output:

```
Hello from a static binary!
```

For a project, the build plan is a file called `build.cljgo` — your
Makefile, written in the same language as the program:

```clojure
(defn build [b]
  (let [app (exe b {:name "newapp"
                    :main "src/newapp/core.cljg"})]
    (install b app)
    (run b app)))
```

No separate configure step, no toolchain variables, no dependency rules to
maintain.

## The honest performance note

Your code becomes Go source, and Go compiles it. So: fast startup, no
interpreter in the binary, real machine code — but a garbage collector, a
scheduler, and boxed values in the general case. This is Go-class
performance, not C-class. Tight numeric loops and hard real-time
deadlines are still C's job.

What you get instead is the deployment story you already like — one file,
a few megabytes, nothing to install — with far less of the program spent
on memory management and string handling.

Ready for the language itself? Start at **1. Hello World** — it assumes
nothing and moves one idea at a time.
