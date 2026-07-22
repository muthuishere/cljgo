## Why

Two dual-mode divergences ship on `main` today: a third-party `go-require`
member returns `nil` under `cljgo run` but the real value from the compiled
binary (both exit 0), and an entry namespace's `*file*` reads `NO_SOURCE_FILE`
in an AOT binary but the real path in the interpreter. Silent REPL-vs-binary
divergence is the failure mode CLAUDE.md calls unforgivable, and it gates ADR
0048 (dependency resolution) and ADR 0054 (publish), whose "never silent `nil`"
guarantees are exactly this invariant. ADR 0053 decides the fix; spikes S30/S31/
S32 diagnosed it and S36 proved the mechanism.

## What Changes

- Third-party `go-require` member access in the interpreter **hard-errors**
  instead of silently returning `nil` — keyed off a reflect-seed registry miss
  on a domain-dotted import path, gated by a new `Evaluator.HostUnlinkedTolerant`
  flag (default `false` = `run`/REPL error; the emitter sets `true` so the AOT
  namespace-discovery pass, which runs the same forms through the interpreter,
  still works). **BREAKING** for any program that today relies on the silent
  `nil` (it now errors — which is the point).
- Entry-namespace `*file*` in an AOT binary binds to the **logical source path**
  (not `NO_SOURCE_FILE`), and entry-namespace `require` of a namespace not
  compiled into the binary **hard-errors** rather than no-op'ing behind the
  provider registry.
- A **dual-harness parity conformance gate** asserting one of three accepted
  outcomes — identical output; identical error; or the interpreter hard-errors
  naming an unavailable capability *and* the AOT leg succeeds — forbidding only
  the silent "different non-error values" quadrant.

## Capabilities

### New Capabilities
- `host-resolution-parity`: the invariant that a host reference the two
  execution legs resolve differently must hard-error in the leg that cannot
  satisfy it, never silently become `nil`/`""`/`false`/a no-op — plus the
  `HostUnlinkedTolerant` mechanism, the entry-namespace `*file*`/`require`
  behavior, and the three-outcome dual-harness parity gate that enforces it.

### Modified Capabilities
<!-- No existing OpenSpec capability's requirements change; this is net-new. -->

## Impact

- `pkg/eval` (`host.go` unlinked-member detection + the `HostUnlinkedTolerant`
  flag on `Evaluator`; entry-namespace `*file*`/`require`), `pkg/emit`
  (`compile.go`/`module.go` set tolerant during discovery; bind entry `*file*`).
- New conformance harness assertion supporting the three-outcome parity check
  (extends the ADR 0007 dual harness).
- Frozen reference: `spikes/s36-unlinked-goref-detection/prototype.patch`.
- Unblocks ADR 0052 and 0050 implementation.
