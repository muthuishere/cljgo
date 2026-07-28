---
title: "Which HTTP thing do I use?"
sidebar:
  label: "Which HTTP thing do I use?"
  order: 2
---

There are three HTTP tools in cljgo and they don't overlap. The first
question splits them cleanly:

**Are you calling someone, or is someone calling you?**

- calling out → the **client**
- being called, small → the **raw server**
- being called, and it's a real app → the **framework**

## Start small: calling someone else's API

If you just want to fetch something, that's the client. One function, a
URL, a map back.

```clojure
(require '[cljg.net.http :as client])

(def reply (client/get "https://example.com"))

(println (:status reply))
(println (subs (:body reply) 0 60))
```

Output:

```
200
<!doctype html><html lang="en"><head><title>Example Domain</
```

`get`, `post`, `put`, `delete`, `patch`, `head` all work the same way:
URL, optional options map, response map back. Nothing here has an opinion
about your program.

## Being called: the raw server

Now flip it around. The raw server is one port and one function. The
function gets a request map and returns a response map — you write
everything in between.

Here's a `/hello/<name>` endpoint, done by hand:

```clojure
(require '[cljg.http :as http])
(require '[cljg.net.http :as client])

(def server
  (http/serve
    {:port 0
     :handler (fn [req]
                (let [name (subs (:uri req) (count "/hello/"))]
                  {:status 200
                   :body   (str "Hello " name "!")}))}))

(def reply
  (client/get (str "http://127.0.0.1:" (:port server) "/hello/asha")))

(println (:status reply))
(println (:body reply))

(http/stop server)
```

Output:

```
200
Hello asha!
```

Look at what that `subs` is doing: chopping the name out of the URL with
string surgery. That works for one route. Add a second route and you're
writing a router. Add a third and you're writing a bad one.

That's the honest boundary of the raw server — and it's exactly right for
a webhook receiver, a health endpoint, a metrics port, or a service with
two URLs.

## Being called, for real: the framework

Same task, one layer up:

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

The string surgery is gone: `{name}` in the path, `param!` to read it.
The `GET` is explicit, so a `POST` to the same path won't land here by
accident. `ok` writes the `200` for you.

And that log line you didn't ask for is the point. Framework means
request logging, request IDs, error recovery, CORS, `/healthz` — on by
default, because that's what a real app needs and nobody enjoys wiring it
up a fourth time.

## Put it together: the decision

| your situation | use |
|---|---|
| fetching a URL, calling an API | `cljg.net.http` (client) |
| one or two endpoints — webhook, health check, metrics | `cljg.http` (raw server) |
| routes, HTML pages, logins, middleware | `bri.web` (framework) |
| you're a library and don't want to impose a framework | `cljg.http` |

The two servers are not rivals: **the framework is built on the raw
server.** `bri.web` parses your route table, stacks your middleware, and
then hands one handler function to `cljg.http/serve` — the same call you
just wrote by hand.

So starting raw costs you nothing. When the second route shows up, move
up a layer.

Next: [where to keep secrets](/cljgo/choosing/secrets/) — because the
first thing a real server needs is an API key it isn't allowed to
hard-code.
