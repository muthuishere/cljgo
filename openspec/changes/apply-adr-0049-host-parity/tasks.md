## 1. Unlinked third-party go-require → hard-error

- [ ] 1.1 Add `HostUnlinkedTolerant bool` field to `Evaluator` (`pkg/eval`), default false
- [ ] 1.2 At the two `host.go` unlinked sites (`:27`, `:62`), replace `return nil, nil` on a registry miss for a domain-dotted path with: if `HostUnlinkedTolerant` keep nil, else return the ADR-0049 error (module path + member + `*file*`)
- [ ] 1.3 Confirm detection keys off the `corelib.LookupHostMember` `ok` flag on an `isThirdPartyGoPath`, so stdlib/cljgo-own hits and genuine-nil values are untouched
- [ ] 1.4 Set `HostUnlinkedTolerant = true` at the emitter's namespace-discovery entry point (`pkg/emit` compile/module), so `cljgo build` of a third-party-go-require program still discovers namespaces

## 2. Entry-namespace `*file*` and `require` in a binary

- [ ] 2.1 Bind the entry namespace's `*file*` to its logical source path during emission (not `NO_SOURCE_FILE`)
- [ ] 2.2 Make binary `require` of a namespace not compiled into the binary hard-error (naming the namespace) instead of relying on the provider registry

## 3. Dual-harness parity gate

- [ ] 3.1 Add a comparator to the ADR-0007 dual harness that accepts {identical output} ∪ {identical error} ∪ {interpreter capability-error AND AOT success} and fails on different-non-error-values or silent-nil-vs-value
- [ ] 3.2 Seed parity cases from the S26/S27 third-party-go-require repro and the S25 entry-`*file*` repro

## 4. Tests and gates

- [ ] 4.1 Unit test: unlinked member errors under run/REPL, is tolerated under the emitter flag, stdlib/genuine-nil unaffected (adapt S31's fixtures)
- [ ] 4.2 Build test: a program using a third-party go-require compiles (discovery tolerates) and the binary resolves the real value
- [ ] 4.3 Conformance/dual-harness cases for the entry-`*file*` and uncompiled-`require` behaviors
- [ ] 4.4 Full gates green: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...`

## 5. Close-out

- [ ] 5.1 Verify no spike code merged into `pkg/`; `prototype.patch` stays a reference only
- [ ] 5.2 Move ADR 0049 note to reflect implementation landed; `/opsx:archive` this change
