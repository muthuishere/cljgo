---
title: Dual-host `.cljc` projects
description: One .cljc source tree that runs on JVM Clojure and on cljgo — layout, project files, source-root precedence, the REPL, and how to test both hosts so they cannot drift.
---

`.cljc` is how one source tree targets two hosts. Write the library once, run
it on JVM Clojure **and** on cljgo, publish it to Clojars, and let consumers
on either host use it.

This guide is what a dual-host project actually needs. It is written from two
real ones — [koine](https://github.com/muthuishere/koine) (a host-access seam
library) and the toolnexus Clojure port — including the mistakes they hit.

## Layout

Nothing exotic. A normal Clojure library:

```
myproj/
  deps.edn            ← JVM: source roots and dependencies
  build.clj           ← JVM: tools.build, for publishing to Clojars
  src/myproj/core.cljc
  test/myproj/core_test.cljc
```

`build.clj` is **tools.build's**, not cljgo's. cljgo does not read it and will
not try to — it probes `build.cljgo` then `build.cljg` only. That separation
matters: a JVM build script that requires `clojure.tools.build.api` is
unloadable on cljgo, so a version of cljgo that claimed the name broke every
project of this shape.

## Which project file does cljgo read?

**`build.cljgo` first, `deps.edn` second.**

| you have | cljgo reads |
|---|---|
| `build.cljgo` | `build.cljgo` — source roots and dependencies |
| `deps.edn` only | `deps.edn`'s `:paths` |
| both | `build.cljgo`, always |

So a library that already declares its roots on the JVM needs **no second
project file**:

```clojure
;; deps.edn
{:paths ["src"]
 :deps  {org.clojure/clojure {:mvn/version "1.12.5"}}}
```

cljgo reads `:paths` from that and nothing else — not `:deps`, not
`:aliases`, not `:extra-paths`. Those carry tools.deps semantics cljgo does
not implement, and honouring them halfway would produce a load path that
agrees with neither host. Dependencies for cljgo are declared in
`build.cljgo` (see [Dependencies & publishing](/cljgo/guides/deps-publish/)).

Add a `build.cljgo` when you want cljgo to resolve dependencies, build a
binary, or use source roots that differ from the JVM's. Because it wins, it
is also how you opt **out** of a `:paths` that is stale or JVM-only.

## Running it

```bash
clojure -M:test          # JVM
cljgo test               # cljgo, interpreted
cljgo test --compiled    # cljgo, AOT-compiled — a real binary
cljgo repl               # a REPL that can see your project
```

`cljgo test --compiled` is the one worth building the habit around. It
compiles your suite to a binary and runs that, which is the only way to catch
a divergence between the interpreter and compiled code. cljgo treats such a
divergence as a release blocker; your suite should too.

## Testing both hosts, so they cannot drift

The point of `.cljc` is identical behaviour. That only stays true if
something checks it, and the check has to be a *comparison* — not two suites
that each report green.

koine's `run-conformance.sh` is the minimal shape: run the same file on every
host and print each result on its own line.

```
== process_check ==
jvm        48/48 pass
cljgo      48/48 pass
```

Two traps worth inheriting rather than rediscovering:

**Compare the legs; don't just run them.** A suite can be green on each host
and still disagree — one reporting 153 tests and the other 154. Only a
cross-leg comparison catches that. Both legs saying `OK` is not evidence they
agree.

**Never grep stdout for a success marker.** The cljgo REPL prints errors to
**stderr** and continues to the next form, so a trailing `(println :OK)`
prints even when everything before it failed. Assert on the suite's own
verdict line and on exit codes. A runner that greps for a marker will report
a green run that resolved nothing.

And keep per-leg logs. An intermittent failure whose stderr went to
`/dev/null` takes its evidence with it.

## Writing portable code

Use `:cljgo` and `:clj` reader conditionals for the parts that genuinely
differ:

```clojure
(ns myproj.core)

(defn now-millis []
  #?(:clj  (System/currentTimeMillis)
     :cljgo (cljg.time/now-millis)))
```

Two rules that save time:

- **A `.cljc` whose real body is `:clj`-only is an error on cljgo** (`R1012`),
  not a silently empty namespace. That is deliberate — half-loading is worse
  than failing.
- **JVM interop is not portable.** `Thread/sleep`, `System/getenv` and friends
  exist only on the JVM. Reach for the `cljg.*` namespaces, or a seam library
  like koine that provides one API over both.

## Publishing

Publish to Clojars with tools.build as you would any Clojure library — your
`build.clj`, unchanged. Consumers on the JVM get it through `deps.edn`;
consumers on cljgo declare it in their `build.cljgo`. See
[Dependencies & publishing](/cljgo/guides/deps-publish/).

Keep the library free of Java-carrying dependencies if you want it usable on
cljgo. A namespace needing Java interop is refused at `require` with a coded
error (`I4002`) rather than half-loading — the gating is per *namespace*, so
the pure namespaces of a mixed library stay usable.

## If a namespace will not resolve

```
error: could not locate namespace myproj.core
(no registered provider, and no myproj/core.clj/.cljg/.cljc
 relative to the requiring file)
```

Read the parenthesis: **"relative to the requiring file"** is the fallback
root, the one used when no project source roots were established. It means
cljgo found no `build.cljgo` and no `deps.edn` `:paths` — not that your file
is missing.

Check, in order:

1. A `deps.edn` with `:paths`, or a `build.cljgo`, at the directory you are
   running from.
2. The namespace matches the path: `src/myproj/my_util.cljc` declares
   `(ns myproj.my-util)` — underscores in the file, hyphens in the name.
3. You are running from the project root, not from inside `src/`.
