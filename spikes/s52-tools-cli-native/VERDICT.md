# s52 VERDICT — MET

**Date:** 2026-07-27 · **Spike:** s52-tools-cli-native · **ADR:** 0096

## Question

Can `org.clojure/tools.cli` 1.1.230 be ported into cljgo as a **faithful,
near-verbatim** native satellite (`clojure.tools.cli`), with its public API,
option-spec keys, and byte-exact outputs preserved against the real JVM library?

## Outcome: MET

The port is the upstream single-file source with reader conditionals resolved
and **three behavior-preserving adaptations** (ns→satellite preamble; `#?`
branches inlined to `:clj`/`:default`; `:post` assertion map → explicit
`assert`s). No namespace is scoped out — the library is fully pure.

### Evidence

- **15 representative behaviors + the legacy `cli` banner are byte-identical**
  between a cljgo run of the port and the JVM oracle (Clojure 1.12.5 +
  tools.cli 1.1.230). Covered: `parse-opts` happy path, boolean toggles,
  clumped short opts (`-vvv` via `:update-fn`), `--[no-]` negation,
  `:validate` failure, unknown option, missing required arg, `:missing`
  required option, `:in-order` subcommand stop, `--opt=val` assignment,
  `:multi`/`:update-fn conj`, `--` terminator, summary column layout,
  `get-default-options`, and legacy `cli`. Frozen in
  `draft-conformance-tools.cli.clj`.
- Every cljgo primitive the port needs was individually confirmed present:
  `format`, `re-find`/`re-seq`, `condp`, `with-out-str`, `pr-str`,
  `clojure.string/{join,split(limit),replace(regex),trimr,starts-with?}`,
  `*err*`, `binding`, type hints, `^{:private true}` / `defn-`, `ex-info`.

### Honest scope that lands in tier-1

- **Full public API**: `parse-opts`, `summarize`, `make-summary-part`,
  `format-lines`, `get-default-options`, and the deprecated legacy `cli`.
- **All option-spec keys** unchanged (`:id :short-opt :long-opt :required
  :desc :default :default-desc :default-fn :parse-fn :assoc-fn :update-fn
  :multi :post-validation :validate :validate-fn :validate-msg :missing`) and
  all parse-opts flags (`:in-order :no-defaults :strict :summary-fn`).
- **The three adaptations are equivalences, not reductions** — no feature is
  dropped from tier-1.

## Residual unknowns (carried to apply, not blockers)

1. **`:parse-fn` exception strings are host-specific.** `(str e)` for a thrown
   parse-fn embeds the JVM class/message; cljgo's differs. No conformance
   behavior freezes such a string. Integrator should add ONE cljgo-oracle'd
   parse-error case rather than porting a JVM string.
2. **`compile-option-specs` `:post` → `assert`.** Faithful in behavior, but the
   thrown value is cljgo's assert error, not a JVM `AssertionError`. If a future
   ADR wants pre/post maps supported natively, this rewrite can be reverted.
   Not a tier-1 gap (these guard programmer error, not user input).
3. **The dev-time unknown-key warning** (`select-spec-keys`, gated by
   `*assert*`, writes to `*err*`) was kept verbatim but not exercised in the
   freeze — low-risk, integrator may add a `:no-such-key` case.
4. **`load-file` is unbound under `cljgo run`** — surfaced here as a driver
   limitation only; the real loader (integrator step) does not use it.

**Per ADR 0027 §2 this spike is closed on integration.** The port is ready to
land as `core/tools_cli.cljg` with the standard satellite plumbing.
