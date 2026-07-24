# Spike s46 — VERDICT: adopt the Charm stack for bri.cli's interactive backend

Date: 2026-07-24 · Feeds ADR 0078 §7 (the deferred backend decision).

## Recommendation

**Use the Charm stack** (`huh` for prompts/forms, `bubbletea`+`lipgloss` for
progress/TUI). It passes every mandatory gate, its cost is trivial and opt-in,
and it delivers best-in-class UX for ~1 line of integration per widget —
whereas "build our own" means reimplementing a TUI framework for a worse result.
The owner's real ask (*"the very best"*) points here; *"if we want we can build
our own"* is measured below and rejected on evidence.

## Measured (Apple M5 Pro, go1.26.3, `CGO_ENABLED=0 -s -w -trimpath`)

| Gate | Charm | Bespoke (minimal) |
|---|---|---|
| **Pure-Go / `CGO_ENABLED=0` host build** | ✅ | ✅ |
| **cgo packages in closure** | **0** | **0** |
| **Cross-compile** (5-target ADR 0077 matrix) | ✅ all 5 | ✅ all 5 |
| **Marginal binary cost** (over an empty Go binary) | **+2.66 MB** | +0.66 MB |
| **Dependency modules** | 47 | 3 (`x/term`,`x/sys`) |
| **Integration LOC** (to use every widget) | **91** | 177 |
| **Widget coverage** | text · **filterable select** · **multiselect** · confirm · password · **inline validation** · **multi-field forms** · spinner · theming · resize/`NO_COLOR` | text · password · **basic** select (no filter/paging) · confirm · spinner |

## Reading the numbers

- **Mandatory gates: both pass.** Charm is NOT disqualified — the whole
  `bubbletea`/`huh`/`lipgloss` stack is pure Go (its `mattn/go-isatty` +
  `go-runewidth` deps are pure Go; only the unrelated `mattn/go-sqlite3` is
  cgo), so a Charm-backed bri.cli app still AOT-compiles to a `CGO_ENABLED=0`
  static binary and still cross-compiles via `cljgo dist`.
- **Cost is trivial and opt-in.** +2.66 MB is nothing for a CLI, and ADR
  0074/0076 opt-in linking means it lands **only** in binaries that require
  `bri.cli` — a web app or a plain program pays zero. The 47 modules ride the
  same opt-in boundary; they never touch a non-CLI binary.
- **The bespoke result argues against itself.** 177 LOC bought a *minimal*
  kit — basic arrow-key select (no filtering, no paging, no theming), and it is
  **missing** multiselect, multi-field forms, an editor for `:multiline`, and
  inline-validated redraw. Reaching Charm's quality means implementing exactly
  what `bubbletea` already is (the Elm loop, escape-sequence parsing, resize
  handling, a component library) — thousands of LOC and permanent maintenance,
  for a worse result. That is the opposite of "make the CLI better first."
- **The unified model maps cleanly.** Each widget is one `huh` call keyed by the
  param `:type` (`:string`→Input, `:enum`→Select, `:multi`→MultiSelect,
  `:bool`→Confirm, `:secret`→password Input, `:multiline`→Text), and huh's
  `.Validate(fn)` takes the **same** validator the CLI arg path runs (ADR 0078
  §3) — so a bad prompt answer re-asks with the validator's own message, from
  one declaration. Non-TTY degrades to the flag/default path (the resolution
  pipeline already handles it).

## Integration plan (bri.cli increment 2)

- Flip `bri.cli` to **OptIn** (ADR 0074/0076): its Go shims move to an isolated
  `pkg/bri/cli` package that imports huh/bubbletea/lipgloss and
  `RegisterInstaller`s; genbri emits the opt-in `provider.go`; the emitter
  blank-imports `pkg/briaot/bricli` only when an app requires `bri.cli`. `bri.cli.validate`
  stays pure-Clojure in the umbrella (no deps).
- The interpreter's `briloader` blank-imports `pkg/bri/cli` (dev links
  everything; the zero-cost guarantee is a user-binary property).
- Wire the prompt into the resolution pipeline's slot 3 (`flag → env →
  prompt-if-TTY → default → error`, ADR 0078 §2): a `-cli-prompt` shim keyed by
  `:type`, running the param's validators via huh's `.Validate`.
- A `go list -deps` zero-cgo test on `pkg/briaot/bricli` (mirroring
  s45/ADR 0074/0076 proofs) keeps the pure-Go guarantee CI-enforced.

## Not chosen

- **Bespoke renderer** — measured; smaller but far less capable, and matching
  Charm is a framework-sized project. Rejected on quality + maintenance.
- **A third-party non-Charm TUI lib** — Charm is the ecosystem standard, pure-Go,
  actively maintained, and already validated here.
