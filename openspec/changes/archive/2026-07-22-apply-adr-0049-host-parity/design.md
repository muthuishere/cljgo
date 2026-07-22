## Context

ADR 0049 is the authority; this is its implementation design. Spike S36
(`spikes/s36-unlinked-goref-detection/`, PASS) established the mechanism and
froze a working `prototype.patch`; S30/S31/S32 diagnosed the divergences. The
silent `nil` is an explicit `return nil, nil` at two sites in
`pkg/eval/host.go` (`:27`, `:62`), reached only by a reflect-seed registry miss
(`corelib.LookupHostMember`) for a domain-dotted import path
(`isThirdPartyGoPath`). `ResolveLibPath` is the single shared resolver, so the
interpreter and emitter agree by construction.

## Goals / Non-Goals

**Goals:**
- Eliminate the third-party `go-require` `nil`-vs-value divergence: the
  interpreter hard-errors; the AOT binary keeps working.
- Fix entry-namespace `*file*` and uncompiled-`require` divergence in binaries.
- Add a reusable three-outcome dual-harness parity gate so a regression is
  caught in CI.

**Non-Goals:**
- Making third-party Go usable at the REPL (that is the design/05 self-rebuild
  capability; this only makes the *absence* honest).
- General REPL-vs-binary equivalence (timing, ordering, GC).
- Any `pkg/` change from the spikes themselves — spike code never merges (ADR
  0027); `prototype.patch` is a reference, re-authored here with tests.

## Decisions

1. **`Evaluator.HostUnlinkedTolerant bool`** (default `false`). At the two
   `host.go` sites, when the registry misses on a domain-dotted path: if
   tolerant, keep today's `nil` (discovery pass); else hard-error with the
   ADR-0049 message. The emitter (`pkg/emit` compile/module discovery) sets it
   `true`; `run`/REPL leave it `false`. Detection is a registry miss — structural,
   not a heuristic — so a genuinely-`nil` value (never routed through `evalHost`)
   and a linked-returns-`nil` (registry hit) are both unaffected.

2. **Lazy at member access**, not eager at `require-go`: the error names module
   + member + `file` and fires only for members actually used.

3. **Entry `*file*`**: bind the entry namespace's `*file*` to its logical source
   path during emission (not `NO_SOURCE_FILE`); make binary `require` of an
   uncompiled namespace hard-error instead of relying on the provider registry.

4. **Parity gate**: extend the ADR-0007 dual harness with a comparator that
   passes on {identical output} ∪ {identical error} ∪ {interpreter
   capability-error AND AOT success}, and fails on different-non-error-values or
   silent-`nil`-vs-value. Seed it with the S31/S32 and S30 repro cases.

## Risks / Trade-offs

- **BREAKING**: programs relying on the silent `nil` now error under `run`. This
  is intended and is the entire point; documented in the error message with the
  remedy (`cljgo build` / self-rebuild).
- **Tolerant-flag plumbing**: the flag must reach every discovery-pass evaluator
  or a legitimate build could spuriously error. Mitigation: set it at the single
  emitter entry point and cover with a build test over a third-party-`go-require`
  fixture (S31 has one).
- **Entry `*file*` value**: a binary has no source tree at runtime, so `*file*`
  is a logical path, not an on-disk one — semantics match, not byte-identity.
  Acceptable and documented.
