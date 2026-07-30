---
title: "Serve HTTP"
sidebar:
  label: "7. Serve HTTP"
  order: 7
---

A web server in cljgo is a function. It receives a request as a map and
returns a response as a map. That's the entire model — everything else is
detail.

## Start small: the smallest server

```clojure
(require '[cljg.http :as http])

(http/serve {:port 8080
             :handler (fn [req]
                        {:status 200
                         :body   "Hello from my first server!"})})
```

Visit `http://localhost:8080`:

```
Hello from my first server!
```

Read the `serve` call with page 3 eyes: it's one options map — `:port`,
and a `:handler` function. The handler ignores the request and always
answers the same thing.

## Look at the request

The request is a plain map. Pull out `:uri` to see what was asked for:

```clojure
(http/serve {:port 8080
             :handler (fn [req]
                        {:status 200
                         :body   (str "you asked for " (:uri req))})})
```

Now `http://localhost:8080/fruit/mango` answers:

```
you asked for /fruit/mango
```

No router yet, no annotations, no framework — just reading a key from a
map.

## Put it together: test your server from code

`:port 0` means "pick any free port" — perfect for tests. The returned
server tells you where it landed, and `stop` shuts it down gracefully:

```clojure
(require '[cljg.net.http :as client])

(def server
  (http/serve {:port 0
               :handler (fn [req] {:status 200 :body "pong"})}))

(def reply (client/get (str "http://127.0.0.1:" (:port server))))

(:status reply)
(:body reply)
```

Output:

```
200
pong
```

```clojure
(http/stop server)
```

Server up, request made, answer checked, server down — in one file, no
tools outside cljgo.

When you want routing, middleware, HTML and auth on top of this
primitive, that's the **bri** framework — built on exactly this `serve`.
