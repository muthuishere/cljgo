# The fifteen minutes — your first keel app

keel is cljgo's application framework (ADR 0041): the batteries of
Spring Boot, the manners of a library — you call it, it never calls
you. Nothing is scanned, nothing is ambient; every generated file is
yours.

## 1. Generate

```
$ cljgo new myapp
$ cd myapp
```

You get the blessed layout, all plain files:

```
src/app/main.cljg    the app — THE golden page, T1 edition
conf.edn             config: one EDN map (+ :profiles)
conf.schema.edn      optional; enforced because it exists
public/app.css       a real stylesheet, served from disk (#NOBUILD)
test/app/main_test.cljg  one passing test (in-process http client)
build.cljgo          the build plan (ADR 0021)
.gitignore
```

## 2. Run

```
$ cljgo dev
keel dev — myapp
  profile : dev
  nREPL   : nrepl://127.0.0.1:57123 (.nrepl-port written)
  reload  : re-(def) a handler var at the REPL — routes hold #'vars

keel: listening on http://localhost:3000
```

Open http://localhost:3000 — a styled page, CSS from `public/`, no
pipeline. `cljgo dev` also:

- attaches an **nREPL** (the `.nrepl-port` file is the editor
  convention) — connect from your editor;
- turns on **dev warnings**: a route holding a plain fn (instead of a
  `#'var`) is called out, because it silently loses liveness;
- serves with the **default middleware stack** — access-log, recover
  (the error funnel), sessions (signed cookies), JSON negotiation,
  CSRF. Security is what you didn't type.

## 3. Change it live

Routes hold **vars**, and vars deref per request. From your connected
REPL:

```clojure
(in-ns 'app.main)
(defn home [_req]
  (http/ok (html/page [:h1 "hello from the REPL"])))
```

Refresh the browser. No restart, no reload machinery — the running
server sees the new definition because `#'home` is on the routes page.

## 4. Read the page you own

`src/app/main.cljg` is under a page and is the whole app:

```clojure
(ns app.main
  (:require [keel.http :as http]
            [keel.html :as html]
            [keel.config :as config]))

(def cfg (config/load!))          ; conf.edn + APP_* env. Reads a file, no more.

(defn home [_req]
  (http/ok (html/page {:title "myapp"} [:h1 "It's alive."])))

(def routes
  [["GET /{$}"     #'home]
   ["GET /static/" (http/dir "public")]
   ["GET /health"  (http/health {})]])

(defn -main [& _args]
  (http/serve routes {:port (:port cfg)}))
```

The shape is the doctrine: **top-level defs construct values, with no
I/O** (requiring `app.main` is side-effect-free — that's why tests can
load it); `-main` starts the world; routes are data on Go's own
ServeMux patterns; the safe middleware stack applies because you
didn't type one.

## 5. Test

```
$ cljgo test
Ran 1 tests containing 2 assertions.
0 failures, 0 errors.
```

The generated test uses the in-process client — same mount, same
middleware, no socket:

```clojure
(deftest home-page-renders
  (let [res (http/request main/routes {:method "GET" :path "/"})]
    (is (= 200 (:status res)))))
```

## 6. See what you're running

```
$ cljgo config      ; every key, its value, and the layer that won
$ cljgo routes      ; every route + the effective middleware stack
```

## Where next

- [keel.http](keel-http.md) — handlers, middleware, the error funnel
- [keel.html](keel-html.md) — pages, forms, escaping
- [keel.config](keel-config.md) — profiles, env, the schema

The data layer (`keel.db`, migrations, the embedded dev Postgres) and
jobs/cache land in the next tiers of the app-framework change.
