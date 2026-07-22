# Spike S36 — Can the interpreter tell an UNLINKED third-party `require-go` member from a legitimate `nil`?

Opened 2026-07-22. Validates **ADR 0053 decision 2** (the one uncertain
mechanism keeping that ADR at `draft`). Follows the diagnosis spikes
S31 / S32, which **already measured** that the divergence exists — this
spike does NOT re-litigate that; it proves whether the FIX's detection
predicate is reliable.

## The one question

When `(some.pkg/Member)` is evaluated after
`(require-go '["github.com/x/y" :as some.pkg])`, can the interpreter
**reliably** distinguish:

- **(a) unlinked third-party member** — a member of a third-party Go
  package that is NOT linked into the running `cljgo` binary. Today this
  returns `nil` silently while the compiled binary returns the real value
  (the unforgivable REPL-vs-binary divergence). ADR 0053 dec. 2 says this
  must become a **hard error**.
- **(b) a legitimately-`nil` Clojure value** — must NOT error.
- **(c) a stdlib or cljgo-own Go symbol that resolves fine** — must NOT
  error, must return its real value.

...WITHOUT false-erroring on (b) or (c) — AND without breaking
`cljgo build`, whose namespace-discovery pass evaluates the very same
member-access forms through the interpreter (S31 §2.1, S32 §1.3).

## Exit criterion (written BEFORE any code, per ADR 0027)

With the prototype fix in place, all THREE verified with real captured
command output:

1. **(a)** A program that `require-go`'s a third-party module and calls a
   member: the interpreter (`cljgo run` / REPL) raises a **clear error
   naming the module path AND the member** — not `nil`, non-zero exit.
2. **(c)** A program using only stdlib `require-go` (strings/strconv/math):
   **no false error**, byte-identical output to today.
3. **(b)** A program that produces a genuinely-`nil` Clojure value
   (`nil`, `(get {} :x)`, a `nil`-returning fn): **no false error**,
   `nil` prints as `nil`.

Plus the build-parity guard:

4. **`cljgo build`** of the same third-party program still succeeds (the
   emitter's interpreted discovery pass must TOLERATE the unlinked access,
   because the emitted binary links it for real) — proving the fix is
   mode-aware, not a blanket hard error.

Anything less — the predicate false-errors on (b)/(c), or the hard error
breaks `cljgo build` — closes this spike **no**, and ADR 0053 dec. 2 must
find a different mechanism.

## What must additionally be investigated and reported

1. **What the interpreter knows at member-access time** — the exact point
   `nil` is produced, and what distinguishes it from a real `nil`.
2. **The detection predicate** — eager (error at `require-go`) vs lazy
   (error at first member access): which gives the better message and
   avoids false positives / breaking `cljgo build`.
3. **False-positive guards** — enumerate any ambiguous case where a real
   `nil` is indistinguishable from an unlinked-member `nil`.
4. **Eager option** — should the interpreter hard-error at `require-go`
   time instead? Recommend.
5. **Message quality** — must name module path + member (+ file:line).

## Method

Throwaway prototype. Per ADR 0027 + S30's sanctioned method, where
proving the predicate requires patching `pkg/eval/host.go`, the patch is
applied as a **local experiment, measured against the real `cljgo`
binary, then reverted**, and frozen here as `prototype.patch`; the
tracked tree is left clean. All claims in `VERDICT.md` are backed by real
captured output under `results/`.

## Results

See `VERDICT.md`.
