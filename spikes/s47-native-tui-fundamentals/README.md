# Spike s47 — native TUI fundamentals: build our own, portable, opencode-class?

Owner override of s46 (which recommended Charm): *"performance and size is key so
no charm. if we build it our native it can do both for CLOJURE jars and people
also use — if fundamentals are right we can expand … can we do some editor like
opencode?"* This spike tests whether bri.cli can **own** a small, fast, portable
terminal UI whose fundamentals scale from a prompt up to an editor.

## The finding that reframes it

**opencode's TUI is itself Bubble Tea** (the Charm stack), built on the Elm
model→update→view architecture (verified — see VERDICT sources). So
"opencode-class TUI" literally means "owning the Elm loop." Charm is one
implementation of that loop; building our own = building a minimal Bubble Tea.
That is a real but **bounded** investment, and its payoff is exactly the owner's
thesis: smaller, faster, and **portable Clojure** — the same bri.cli runs on
cljgo AND the JVM, which a Go-only Charm dependency can never do.

## What a TUI framework actually is (the fundamentals)

1. a terminal **primitive** — raw mode + read keys + write ANSI (the ONLY
   platform-specific part; cljgo → Go `x/term`, JVM → JLine — ~30 LOC each).
2. an input **event stream** — bytes → Key events.
3. the **Elm loop** — `Model` + `Update(model,msg)→model` + `View(model)→lines`.
4. a **diff renderer** — repaint only changed lines (flicker-free, cheap).
5. **widgets** as plain `(model,update,view)` values composed into the app.

Everything except (1) is **portable logic** — in the real bri.cli it is written
in Clojure and runs on both hosts.

## Layout

- `runtime/` — a from-scratch Go prototype of all five, with TWO widgets (a
  select AND a minimal multi-line editor) to prove the range. `--measure` links
  it all for sizing.
- `VERDICT.md` — measurements, the portability + scale-to-opencode assessment,
  the honest costs, and the recommendation.
