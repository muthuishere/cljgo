# ADR 0005 — Go (value, error) maps to [v err]; `!` suffix throws
Date: 2026-07-11 · Status: accepted (design/05 §2 wins over draft doc/04)

## Context
Go returns errors as values; Clojure throws. Glojure is vector-only (noisy
happy path); let-go throws-only (kills error-as-value idioms).

## Decision
A plain interop call returns [v err] (trailing error/bool detected by TYPE via
go/types in AOT, reflect in eval — identical semantics). The `!`-suffixed call
(os/Open!) unwraps and throws, wrapping the Go error (retrievable via
ex-go-error; errors.Is/As compose). `!` is illegal in Go identifiers, so the
sugar is unambiguous.

## Consequences
Both Go idiom and Clojure idiom stay first-class; one shaping-rule table
shared by both modes (pkg/host).
