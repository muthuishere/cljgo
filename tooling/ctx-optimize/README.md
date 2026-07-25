# cljgo pack for ctx-optimize

Same stance as `../tree-sitter/`: **adopt, don't fork** (ADR 0017 §2). This is
a pack definition over stock `tree-sitter-clojure`, not a grammar.

The pack itself is **generated** — don't edit it by hand:

```sh
go run ./tooling/gen-editors     # → .ctxoptimize/grammars/cljgo.json
```

Its contents come from [`../definers.json`](../definers.json), the same file
that feeds the nvim / Emacs / VS Code configs. See [`../README.md`](../README.md)
for setup commands.

## Why the pack looks unusual

ctx-optimize's normal pack format maps **node type → kind**
(`function_declaration` → `function`). Clojure has no such node type: a
definition is an ordinary `list_lit` whose *head symbol* carries the meaning
and whose *second element* is the name.

Mapping `list_lit → function` produces garbage — every node named after the
macro, and the real names absent entirely:

```
function  defn      ← should be fetch-user
variable  def       ← should be config
                    ← `handler` never appears at all
```

So the pack uses **`decl_rules`** instead, which matches on the head symbol and
reads the name from the next element. Two rules, because `(clojure.core/in-ns
'bri.cli)` — how 34 files in this repo declare their namespace — wraps its name
in a quote and needs `name_unwrap`.

## Adding your own defining forms

Add an entry to `../definers.json` and regenerate. This works because
`LoadPacks` reads `<repo>/.ctxoptimize/grammars/` **before** the machine-wide
directory, so cljgo's definers ship with cljgo — no ctx-optimize release and no
registry entry needed.

That is also why this has to be data rather than grammar: a Clojure program
creates defining macros at run time (`core/bri/cli.cljg` defines `defcommand`),
so no parser can know them, but a table can.

## What it deliberately misses

Everything emitted is read literally from the file; the matcher under-claims
rather than guesses.

- `s/def` and `:rename` aliases — head text no longer matches exactly.
- `(defn ^:private f …)` — metadata in the name slot, so the form is skipped.
- `(defn …)` inside a syntax-quote — a macro *constructing* code, not defining
  it. Excluded via `skip_inside`.

Measured on this repo: 1,372 definer-headed forms, 1,362 resolved to a literal
name, **0 wrong**.

## Requires ctx-optimize with `decl_rules`

Shipped in `2026-07-25-homoiconic-decl-rules`. `ctx-optimize languages add
clojure` now produces a loadable pack directly; this repo-local one exists to
carry bri's definers on top.
