;; ADR 0049 dec 3 — entry-namespace *file* parity (S30 entry-*file* repro).
;; The entry namespace's *file* MUST bind to its logical source path in an
;; AOT binary, matching the interpreter — not NO_SOURCE_FILE. Pre-fix, the
;; compiled leg printed "has-file false" while the interpreter printed
;; "has-file true": a silent REPL-vs-binary divergence. This asserts the
;; property (real path present) rather than an exact path string, so both
;; legs are byte-identical regardless of how the harness names the file.
(println "has-file" (not= *file* "NO_SOURCE_FILE"))
(not= *file* "NO_SOURCE_FILE")
