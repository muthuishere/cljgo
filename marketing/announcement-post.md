# cljgo post — short

**Title:** cljgo — Clojure hosted on Go (0.x, working)

I built Clojure hosted on Go so I can work across my projects with both
ecosystems. Very simple idea: emit Go source like CLJS emits JS. Clojure
libraries and Go libraries are both first-class. Web APIs too — Compojure-style
routes on net/http:

```clojure
(def routes
  (http/routes
    (GET  "/api/hello" #'hello)
    (POST "/login"     #'login)
    (GET  "/api/admin" (auth/admin-only) #'admin)))
```

REPL with nREPL, or `cljgo build` → one static binary, no JVM. Same source,
byte-identical output both ways.

Still 0.x, but it's working — 93% of clojure.core, verified against JVM
Clojure 1.12.5.

https://github.com/muthuishere/cljgo

---

# Slack (#announcements) — small

:new: *cljgo* — Clojure hosted on Go (0.x, working)

Simple idea: emit Go source like CLJS emits JS. Clojure and Go libraries both first-class, web APIs Compojure-style on net/http. nREPL for the REPL side, or `cljgo build` → one static binary, no JVM — same source, byte-identical output both ways.

93% of clojure.core, verified against JVM Clojure 1.12.5.

https://github.com/muthuishere/cljgo
