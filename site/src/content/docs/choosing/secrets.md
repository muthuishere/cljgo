---
title: "Where do I keep secrets?"
sidebar:
  label: "Where do I keep secrets?"
  order: 3
---

A secret is any string that would be bad news in a screenshot: an API
key, a database password, a signing key. The one rule is that it never
lives in your source code. Where it lives instead depends on where the
program runs.

Every value on this page is an obvious fake. Yours should never appear in
a doc, a log, or a terminal.

## Start small: an environment variable

The oldest answer, and still the right one on a server:

```clojure
(require '[cljg.system :as sys])

(println (sys/getenv "DEMO_API_KEY"))
```

Run with the variable set:

```
DEMO_API_KEY=sk-fake-12345678 cljgo run app.clj
```

Output:

```
sk-fake-12345678
```

Simple, works everywhere, and every deploy platform knows how to set one.
The catch: a `getenv` returns a plain string, and plain strings end up in
log lines, error messages and stack traces by accident.

## One step up: a secret that won't print itself

`cljg.secrets` returns a **masked** value. It reads the same sources, but
what comes back refuses to reveal itself until you explicitly ask.

```clojure
(require '[cljg.secrets :as secrets])

(def api-key (secrets/get "env://DEMO_API_KEY"))

(println api-key)
(println (count (secrets/reveal api-key)))
```

Output:

```
{:cljg.secrets/secret true, :masked len=16 ***…78}
16
```

The first line is an accidental `println` of a secret — and it printed
nothing useful. The plaintext only comes out of `reveal`, which means you
can grep your codebase for `reveal` and see every place a secret is
unwrapped. That's the whole design.

`env://NAME` is one *store*. You can also give a list, tried in order —
first hit wins:

```clojure
(def api-key
  (secrets/get ["keychain://my-app/api-key"
                "env://DEMO_API_KEY"]))

(println api-key)
```

Output:

```
{:cljg.secrets/secret true, :masked len=16 ***…78}
```

One line that says: *use my laptop's keychain if it's there, otherwise
fall back to the environment.* Same code on your machine and in
production.

## The keychain: don't put it in a file at all

On your own machine there's a better place than a `.env` file — the
credential store your OS already ships. `secrets/set` writes there by
default:

```clojure
(require '[cljg.secrets :as secrets])

(secrets/set "cljgo-book-demo" "api-key" "sk-fake-abcdefgh")

(def found (secrets/get "cljgo-book-demo" "api-key"))

(println found)
(println (secrets/reveal found))

(secrets/delete "cljgo-book-demo" "api-key")

(println (secrets/get "cljgo-book-demo" "api-key"))
```

Output:

```
{:cljg.secrets/secret true, :masked len=16 ***…gh}
sk-fake-abcdefgh
nil
```

Two names identify a secret: a **service** (your app) and a **name** (the
key). Deleting something that isn't there is a no-op, so cleanup is safe
to re-run.

## Underneath: the keychain primitive

`cljg.security` owns the actual read and write. Use it directly when you
want plain strings and no masking — and when you want to know *which*
store answered:

```clojure
(require '[cljg.security :as sec])

(println (sec/save-to-keychain "cljgo-book-demo" "db-password" "hunter2-not-real"))

(println (sec/get-from-keychain "cljgo-book-demo" "db-password"))

(println (sec/delete-from-keychain "cljgo-book-demo" "db-password"))
```

Output:

```
:native
hunter2-not-real
:native
```

`:native` means the real OS store handled it — macOS Keychain here, and
equally the Windows Credential Manager or the Linux Secret Service.

**The honest platform note:** a headless Linux server has no keychain
daemon running. There, the same call returns `:file` instead — cljgo
falls back to an encrypted file under your user config directory,
readable only by that machine. It always works; it just isn't always the
OS store, and the return value tells you which you got.

## Put it together: the decision

| where the code runs | use |
|---|---|
| a server, a container, CI | environment variables — via `secrets/get "env://…"` so the value stays masked |
| your own laptop, day to day | the keychain — `secrets/set` / `secrets/get` |
| a program that must run both places | a fallback list: `["keychain://app/key" "env://KEY"]` |
| you need the raw string and the backend name | `cljg.security` keychain functions |

And three rules that never change:

- never commit a secret, not even to a private repo
- never log one — that's what masking is for
- rotate anything that has been printed once; assume it's gone

Next: [which state tool](/cljgo/choosing/state/) — where a fetched secret
gets cached, and who does the work in the background.
