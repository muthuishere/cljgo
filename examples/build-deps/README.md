# build-deps — a project that depends on a local library

Demonstrates dependency resolution (ADR 0052): a consumer app that `(dep …)`s a
pure-Clojure library living outside its own source tree, and resolves it
**identically** under the interpreter (`cljgo run`) and the compiled binary
(`cljgo build`) — one resolver feeds both legs (ADR 0053).

```
build-deps/
  greetlib/            the dependency (a library: no build.cljgo needed)
    src/greet/core.cljg
  app/                 the consumer
    build.cljgo        declares (dep b "greetlib" {:path "../greetlib"})
    src/app/core.cljg  requires greet.core
```

## Run it

```bash
cd app
cljgo build run                 # resolve greetlib, compile the app, run it
#  → Hello, world, from greetlib!

cljgo run src/app/core.cljg     # the interpreter leg, same dependency
#  → Hello, world, from greetlib!    (byte-identical)
```

The first build writes `app/build.lock.edn` — the committed pin. For a local
`:path` dependency it is recorded as a named hole (`:local/unlocked? true`); a
git dependency is pinned by `:git/sha` + a content (tree) hash.

> A dev `cljgo` binary needs the cljgo source tree for its generated `go.mod`
> replace directive — run inside the repo or set `CLJGO_SRC` to the cljgo
> checkout (the same requirement as any other `cljgo build`).
