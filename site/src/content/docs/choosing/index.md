---
title: "How cljgo is organised"
sidebar:
  label: "How cljgo is organised"
  order: 1
---

Everything in cljgo lives in one of three layers. Once you can tell them
apart, you always know where to look — and what to reach for.

- **the language** — Clojure itself, exactly as it works on the JVM
- **the batteries** — files, sockets, servers, crypto, compression, caches
- **the framework** — an opinionated way to build a whole web app

That's the whole map. The rest of this page makes each layer concrete.

## The language

This is plain Clojure: `map`, `str`, `let`, sequences, strings, EDN, tests.
Nothing cljgo-specific — it behaves the same as it does on the JVM, on
purpose.

```clojure
(require '[clojure.string :as str])

(println (str/upper-case "hello"))
```

Output:

```
HELLO
```

If you learned Clojure elsewhere, this layer is already yours.

## The batteries

These are the things every program eventually needs and that Clojure's
standard library never had: reading files, hashing a password, opening a
socket, gzipping bytes, running a subprocess, caching a value.

```clojure
(require '[cljg.security :as sec])

(println (sec/sha256 "hello"))
```

Output:

```
2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
```

They're unopinionated on purpose: one job each, no conventions to buy
into, no app shape assumed. This is cljgo's answer to "batteries
included" — the Bun-style standard library layer.

## The framework

The framework is where opinions live. It knows what a web app *is*:
routes, middleware, HTML, sessions, auth, health endpoints. You give it a
table of routes and it runs the rest.

```clojure
(require '[bri.web.http :as web])

(def app
  (web/routes
    (web/GET "/hello/{name}"
             (fn [req] (web/ok (str "Hello " (web/param! req :name) "!"))))))

(def reply (web/request app {:method "GET" :path "/hello/asha"}))

(println (:status reply))
(println (:body reply))
```

Output:

```
GET /hello/asha 200 0ms
200
Hello asha!
```

Notice the first line — you didn't ask for request logging, and you got
it. That's the deal with a framework: conventions you didn't write, doing
work you didn't write.

(`web/request` runs a request straight through your app without opening a
socket — the easiest way to test a route.)

## The one rule for deciding

> **Would a program that isn't a web app want this exact function?**
> Yes → it's a battery. No → it's the framework.

A filesystem read is useful to a script, a CLI, a game server — battery.
"Reject this request unless the user is an admin" only makes sense inside
an app with users and requests — framework.

The framework is *built on* the batteries. Nothing is hidden from you:
you can always drop a layer down.

## I want to… → use…

| I want to… | reach for |
|---|---|
| uppercase a string, sort a list, parse EDN | the language (`clojure.core`, `clojure.string`) |
| serve a whole web page with routes, HTML and login | the framework (`bri.web`) |
| run a raw HTTP server: one port, one handler function | a battery (`cljg.http`) |
| call somebody else's API | a battery (`cljg.net.http`) |
| hash a password, sign a JWT, make a token | a battery (`cljg.security`) |
| read, write or watch files | a battery (`cljg.io`) |
| talk to SQLite or Postgres | a battery (`cljg.data.cast`) |
| cache an expensive lookup | a battery (`cljg.cache`) |
| run work in the background | a battery (`cljg.jobs`) |
| gzip something | a battery (`cljg.compress`) |
| open a TCP or UDP socket | a battery (`cljg.socket`) |
| keep an API key out of your source code | a battery (`cljg.secrets`) |
| read env vars, exit codes, program arguments | a battery (`cljg.system`) |
| build a command-line tool with subcommands and help | the framework (`bri.cli`) |

You'll notice the batteries win most rows. That's deliberate — you only
reach for the framework when you want its opinions.

The next three pages zoom into the choices people actually get stuck on:
[which HTTP thing](/cljgo/choosing/http/),
[where secrets live](/cljgo/choosing/secrets/),
and [which state tool](/cljgo/choosing/state/).
