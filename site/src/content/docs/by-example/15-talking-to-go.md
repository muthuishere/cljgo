---
title: "Talking to Go"
sidebar:
  label: "15. Talking to Go"
  order: 15
---

cljgo runs on Go, and it doesn't hide that. A Go package is just another
namespace: you require it and call it. No wrapper library, no generated
stub file, no `.h` to write. This is cljgo's headline feature.

## Start small: call a Go function

```clojure
(require-go '[strings])

(println (strings/ToUpper "hello"))
```

Output:

```
HELLO
```

`require-go` is like `require`, but the thing it pulls in is a **Go
package**. After that, `strings/ToUpper` is a function you call like any
other.

Note the capital letters. Go's own naming is kept exactly — it's
`ToUpper`, never `to-upper`. cljgo doesn't rename anything, so the Go
documentation you find online applies word for word.

## More packages, and values too

```clojure
(require-go '[strings])
(require-go '[math])

(println (strings/Repeat "ab" 3))
(println (strings/Contains "mangoes" "go"))
(println math/Pi)
(println (math/Sqrt 144))
```

Output:

```
ababab
true
3.141592653589793
12.0
```

`math/Pi` isn't a function call — it's a Go constant, used in value
position. Numbers cross the border on their own: the Clojure integer `3`
arrives as a Go `int`, and Go's `float64` comes back as a Clojure double.

## Errors are values

Go functions don't throw. They return two things: the answer and an error.
cljgo hands you both, as a two-element vector:

```clojure
(require-go '[strconv])

(println (strconv/Atoi "123"))
(println (strconv/Atoi "banana"))
```

Output:

```
[123 nil]
[0 #object[*strconv.NumError]]
```

`"123"` parsed, so the error slot is `nil`. `"banana"` didn't, so you get
Go's zero value `0` plus the error. Because the slot is `nil` when things
went well, you can test it directly with `if`.

## `!` — "just give me the answer"

Checking every error is right when you can recover. Usually you can't, and
you'd rather it just blow up. Add a `!` to the function name:

```clojure
(require-go '[strconv])

(println (strconv/Atoi! "42"))

(println
  (try
    (strconv/Atoi! "banana")
    (catch Throwable e
      (str "nope: " (ex-message e)))))
```

Output:

```
42
nope: strconv.Atoi: parsing "banana": invalid syntax
```

`Atoi!` returns the value alone, or throws carrying the Go error — which
is exactly the `try`/`catch` from page 12. `!` can't appear in a Go
identifier, so there's no ambiguity about what you meant.

## Objects and methods

Some Go functions hand back a struct with methods on it. Clojure's dot
form calls them:

```clojure
(require-go '[strings])

(def fixer (strings/NewReplacer "colour" "color" "flavour" "flavor"))

(println (.Replace fixer "the colour and flavour"))
```

Output:

```
the color and flavor
```

And `(.-Field x)` reads a field:

```clojure
(require-go '[net/url])

(def u (url/Parse! "https://cljgo.dev/book/interop?page=2"))

(println (.-Host u))
(println (.-Path u))
```

Output:

```
cljgo.dev
/book/interop
```

## Put it together: a config reader

Everything above in one small, real job — read `key = value` lines, turn
numbers into numbers, leave everything else as text:

```clojure
(require-go '[strings])
(require-go '[strconv])

(def settings
  ["port = 8080"
   "workers = 4"
   "name = orders-api"
   "retries = three"])

(defn parse-setting [line]
  (let [[raw-k raw-v] (strings/Split line "=")
        k             (strings/TrimSpace raw-k)
        v             (strings/TrimSpace raw-v)
        [n err]       (strconv/Atoi v)]
    (if err
      [k v]
      [k n])))

(def config
  (into {} (map parse-setting settings)))

(doseq [[k v] (sort config)]
  (println k "->" (pr-str v)))

(println "port doubled:" (* 2 (config "port")))
```

Output:

```
name -> "orders-api"
port -> 8080
retries -> "three"
workers -> 4
port doubled: 16160
```

Look at how the two worlds blend. `strings/Split` returns a Go slice, and
page 3's destructuring takes it apart. `strconv/Atoi`'s `[n err]` pair is
destructured the same way, and the error is just a value you test. The
result is an ordinary Clojure map.

Nothing was converted, wrapped, or declared. That's the whole idea.

**One honest limit:** `cljgo run` and the REPL can only reach the Go
packages that were linked into the cljgo binary itself. When you compile a
project with `cljgo build`, you can pull in any Go module from the wider
ecosystem by naming it in your `build.cljgo` — see the
[interop guide](/cljgo/guides/interop/) for that, plus struct constructors
and third-party modules.
