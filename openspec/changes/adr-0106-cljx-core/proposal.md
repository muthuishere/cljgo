# adr-0106-cljx-core — opt-in ergonomics ("clj extensions")

## Why

ADR 0106, de-risked by spike s68 (prototype runs green, dual-harness verified).
`(swap! todo conj x)` / `(swap! votes update who (fnil inc 0))` are correct but
wordy, and the verb the author wanted (`add`, `bump`) is never the word on the
page — the first thing a Go/Python arrival trips on.

## What Changes

New namespace **`cljx.core`** — pure Clojure, lazy, registered in `bri.Specs()`,
no Go shim, no dependency, zero bytes when unused. Opt-in: `(require
'[cljx.core :refer [add! bump! dbg]])`.

Surface (v1, deliberately small — every entry is a transparent alias):

| fn | expands to |
|---|---|
| `(add! a x)` / `(add! a k v)` | `(swap! a conj x)` / `(swap! a assoc k v)` |
| `(del! a k)` | `(swap! a dissoc k)`, `disj` for sets |
| `(bump! a)` / `(bump! a k)` / `(bump! a k n)` | `(swap! a inc)` / `update` with `(fnil inc 0)` / by `n` |
| `(upd! a k f & args)` | `(swap! a update k f …)` |
| `(put-in! a path v)` / `(upd-in! a path f & args)` | `assoc-in` / `update-in` |
| `(clear! a)` | `(reset! a (empty @a))` |
| `(toggle! a)` | `(swap! a not)` |
| `(dbg x)` / `(dbg label x)` | prints and **returns x** — drops into a pipeline |
| `(dbg-> …)` / `(dbg->> …)` | print each step of a threading pipeline |

## Impact

- New Specs() row + `core/cljx/core.cljg` + embed; regenerated briaot twin.
- Docs: a book chapter teaching the sugar next to the idiom it replaces, plus
  the honest portability note.
- No change to `clojure.core` — the precedence principle is absolute here.
