# s68 — cljx.core ergonomics (ADR 0106)

**Verdict: MET (prototype runs green).** Every helper is a one-line alias
over the exact `swap!` form it replaces — no new semantics, no shim, no
dependency. `prototype.clj` runs end-to-end on cljgo.

## Name space is clear

Checked against cljgo's live `clojure.core`: the transient family
`assoc!` `conj!` `dissoc!` `disj!` `pop!` **is taken** and must never be
shadowed (precedence principle). Free and used here: `add!` `del!` `bump!`
`upd!` `put-in!` `upd-in!` `clear!` `toggle!` `dbg`.

Deliberately avoided `put!`/`take!` — they collide with
`clojure.core.async`, which a web app is likely to require alongside.
Hence `add!` is polymorphic (`(add! a x)` conj, `(add! a k v)` assoc)
instead of introducing `put!`.

## Before → after

```clojure
(swap! todo conj "buy milk")            (add! todo "buy milk")
(swap! counter inc)                     (bump! counter)
(swap! votes update who (fnil inc 0))   (bump! votes who)
(swap! user assoc :city "Chennai")      (add! user :city "Chennai")
(swap! user assoc-in [:prefs :theme] v) (put-in! user [:prefs :theme] v)
(swap! cart dissoc :coupon)             (del! cart :coupon)
(reset! todo [])                        (clear! todo)
(swap! debug? not)                      (toggle! debug?)
```

`dbg` is the sleeper win — prints **and returns**, so it drops into a
threading pipeline without breaking it:

```
prices: [120 45 80]
over 50: (120 80)
total: 200
```

## Open question for the owner

`bump!` vs `inc!` for the counter verb. `inc!` reads closer to `inc` (and
is free), `bump!` reads more like English and avoids implying it is just
`inc` (the map arity uses `(fnil inc 0)`). Prototype uses `bump!`.

## Still to do at implementation

- Register lazily in `bri.Specs()` as `cljx.core` (pure Clojure, no shim).
- Dual-harness conformance proving each helper is EXACTLY equivalent to the
  form it claims to replace.
- Docstrings must show the `swap!` equivalent — the sugar teaches the idiom.
