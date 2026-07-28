# ADR 0106 — `cljx.core`: opt-in ergonomics for the verbose everyday idioms

Date: 2026-07-28 · Status: **proposed** (owner-directed: *"`(swap! todo conj
"buy milk")` … I think we need a better `add!` with variable name and item
which internally uses swap! — that would be good for UX. People should not
feel more verbose."*). Sits in the `cljx.*` developer-experience tier
introduced by ADR 0105.

## Context

Clojure's state idioms are correct but wordy at the point of use. The common
shapes a beginner writes twenty times a day:

```clojure
(swap! todo conj "buy milk")          ; append
(swap! counter inc)                   ; bump
(swap! users assoc :id 7)             ; set a key
(swap! votes update who (fnil inc 0)) ; bump a key
(swap! cart dissoc :coupon)           ; drop a key
```

Every one is `swap!` + a collection fn the reader must decode. Nothing is
*wrong* — but a newcomer reads five different shapes for "change the box",
and the verb they wanted (`add`, `bump`, `put`) is never the word on the page.
This is the single most common "Clojure feels verbose" complaint, and it is
the first thing a Go/Python arrival hits (see the ADR 0104 on-ramps).

## Decision

Ship **`cljx.core`** — a small, **opt-in**, pure-Clojure namespace of
ergonomic verbs that expand to the exact idiom they replace. Not required by
default; you write `(require '[cljx.core :refer [add! bump! dbg]])` when you
want it.

**Three hard rules** (this is what keeps it honest):

1. **Never shadow, never re-mean.** The precedence principle (owner,
   2026-07-12) is absolute: nothing here may shadow or change anything in
   `clojure.core`. Verified taken and therefore off-limits: the transient
   family `assoc!` `conj!` `dissoc!` `disj!` `pop!`. Names must also dodge
   heavily-used namespaces — notably `clojure.core.async`'s `put!`/`take!`.
2. **Every helper is a transparent alias.** Each one must be explainable as
   "this expands to *that*", introduce **no new semantics**, and its docstring
   must show the `swap!` form it replaces — so the sugar *teaches* the idiom
   instead of hiding it. If a helper cannot be explained in one line of
   equivalent Clojure, it does not belong.
3. **No new dependency, no Go shim, lazy + zero-cost when unused.**

### Proposed surface (v1 — deliberately small)

**Atom verbs** — the ask:

| cljx.core | expands to | notes |
|---|---|---|
| `(add! a x & xs)` | `(swap! a conj x …)` | on a map: `(add! a k v)` → `assoc` |
| `(del! a k)` | `(swap! a dissoc k)` / `disj` for sets | |
| `(bump! a)` / `(bump! a k)` | `(swap! a inc)` / `(swap! a update k (fnil inc 0))` | the vote-counter case in one word |
| `(drop-by! a n)` / `(bump! a k n)` | `(swap! a - n)` / update by `n` | |
| `(upd! a k f & args)` | `(swap! a update k f …)` | |
| `(put-in! a path v)` | `(swap! a assoc-in path v)` | |
| `(upd-in! a path f)` | `(swap! a update-in path f)` | |
| `(clear! a)` | `(reset! a (empty @a))` | keeps the collection type |
| `(toggle! a)` | `(swap! a not)` | flags |

**Debugging** — the highest-value non-atom win, borrowed from Rust's `dbg!`:

| cljx.core | behaviour |
|---|---|
| `(dbg x)` | prints `dbg: <value>` and **returns x** — drops into any pipeline |
| `(dbg "label" x)` | labelled variant |
| `(dbg-> …)` / `(dbg->> …)` | print each step of a threading pipeline |

`dbg` is the one people will use most: today debugging a `->>` chain means
breaking it apart or writing `(doto x println)`.

**Deliberately NOT included in v1:** control-flow sugar, string helpers,
collection aliases (`mapv!` etc.), nil-coalescing operators. They add surface
without removing real pain, and each one is a place cljgo code stops looking
like Clojure. Additions require evidence of frequency, not taste.

### The honest cost

Code using `cljx.core` **does not run on JVM Clojure as-is** — it is a cljgo
namespace. Mitigation, and the reason this is acceptable: it is **pure
Clojure with no host dependency**, so it can be vendored into a JVM project or
published as an ordinary library. It is opt-in, so a codebase that values
portability simply never requires it. We must say this plainly in the docs
rather than letting people discover it at port time.

## Consequences

- Newcomers get the verb they were looking for; the docstring still teaches
  the underlying `swap!` form, so they graduate rather than get stuck.
- One more namespace in the `cljx.*` tier — the tier now has a clear meaning:
  *things that improve the experience of writing cljgo, none of them required*.
- Every helper needs conformance (dual-harness) proving it is exactly
  equivalent to the form it claims to replace.
- Risk to watch: sugar creep. The three rules and the "evidence of frequency"
  bar exist to keep v1 from becoming a dialect.

## Process

Prototype (done, see below) → openspec → implement alongside `cljx.test`
(ADR 0105), same lazy `bri.Specs()` registration, same conformance discipline.
