---
title: "Errors"
sidebar:
  label: "12. Errors"
  order: 12
---

Some things go wrong: a division by zero, a file that isn't there, a form
field a user typed nonsense into. In cljgo you handle that with `try` —
and when *you* are the one saying "this is wrong", you throw an error that
carries **data**, not just a sentence.

## Start small: catch anything

```clojure
(println
  (try
    (/ 1 0)
    (catch Throwable e
      "something went wrong")))
```

Output:

```
something went wrong
```

`try` runs the first form. If it blows up, the `catch` runs instead. Note
that `try` is an *expression* — whichever branch ran, its value is the
value of the whole thing. That's why `println` can wrap it directly.

`Throwable` means "any error at all". On cljgo, use `Throwable` — it
catches everything, including arithmetic errors like this one.

## Ask the error what happened

`e` is the error itself. `ex-message` pulls out its message:

```clojure
(defn safe-divide [a b]
  (try
    (/ a b)
    (catch Throwable e
      0)))

(println (safe-divide 10 2))
(println (safe-divide 10 0))
```

Output:

```
5
0
```

A fallback value instead of a crash. Now the message:

```clojure
(try
  (/ 1 0)
  (catch Throwable e
    (println "caught:" (ex-message e)))
  (finally
    (println "this always runs")))
```

Output:

```
caught: Divide by zero
this always runs
```

Two new things. `ex-message` gives you the human sentence. And `finally`
runs **no matter what** — error or no error, caught or not. It's where
you close a file or release a lock.

Use `ex-message`, not `.getMessage`, for reading a message on cljgo.

## Errors that carry data

A string is a poor error. `"user not found"` tells your `catch` block
nothing it can act on — *which* user? `ex-info` attaches a map:

```clojure
(try
  (throw (ex-info "user not found" {:id 42}))
  (catch Throwable e
    (println (ex-message e))
    (println (ex-data e))))
```

Output:

```
user not found
{:id 42}
```

`(ex-info "message" {...})` builds the error; `throw` raises it;
`ex-data` gets the map back out. This is the normal Clojure way to signal
a problem in your own code — no exception classes to define, no
hierarchy, just a map you can destructure.

If nobody catches it, cljgo prints it and exits non-zero:

```clojure
(throw (ex-info "cart is empty" {:cart-id 7}))
```

Output:

```
error: cart is empty
help: run `cljgo explain G5000`
```

## Put it together: a validator that explains itself

The map is where you put a `:type` — a keyword saying what *kind* of
problem this is — plus the offending value. The caller reads them and
writes a good message:

```clojure
(defn check-age [age]
  (when-not (number? age)
    (throw (ex-info "age must be a number"
                    {:type :invalid-field
                     :field :age
                     :value age})))
  (when (neg? age)
    (throw (ex-info "age must not be negative"
                    {:type :invalid-field
                     :field :age
                     :value age})))
  age)

(defn report [age]
  (try
    (println "ok, age is" (check-age age))
    (catch Throwable e
      (let [{:keys [type field value]} (ex-data e)]
        (println "rejected" (name field)
                 "=" (pr-str value)
                 "-" (ex-message e)
                 (str "(" type ")"))))))

(report 34)
(report -3)
(report "old")
```

Output:

```
ok, age is 34
rejected age = -3 - age must not be negative (:invalid-field)
rejected age = "old" - age must be a number (:invalid-field)
```

`check-age` knows the rules. `report` knows how to talk to a human. The
`ex-data` map is the contract between them — and that's page 3's map
destructuring, right inside a `catch`.

Errors are values, so they travel: hand one to a channel, log its map as
JSON, or turn `:type` into an HTTP status code. Next: doing several things
at once, without any of them stepping on each other.
