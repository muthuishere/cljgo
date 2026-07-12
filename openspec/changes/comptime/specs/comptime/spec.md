## ADDED Requirements

### Requirement: comptime evaluates once at compile time and embeds the value
The system SHALL provide a `(comptime <body>)` special form whose body is
ordinary Clojure evaluated exactly once at compile time by the same evaluator
that runs macros; in AOT mode the resulting value SHALL be embedded in the
emitted Go as a literal constant, and in interpreted/REPL mode the body SHALL
evaluate inline at the point of evaluation with identical result semantics.

#### Scenario: AOT embedding
- **WHEN** a namespace containing `(def table (comptime (into {} (map (fn [i] [i (* i i)]) (range 5)))))` is AOT-compiled and the binary runs
- **THEN** `table` is `{0 0, 1 1, 2 4, 3 9, 4 16}` and the emitted Go contains the map as a literal constant with no runtime computation of the squares

#### Scenario: REPL inline evaluation matches
- **WHEN** the same form is evaluated at the REPL
- **THEN** `table` has the identical value, and re-evaluating the form re-runs the body

### Requirement: comptime results must be embeddable
The system SHALL reject, with a positioned compile-time error, any comptime
result that is not readable Clojure data per the embeddability table (nil,
booleans, numbers except NaN/±Inf, strings, chars, keywords, symbols, regex
patterns, and lists/vectors/maps/sets of embeddable values, with metadata);
fns, vars, Go handles, channels, reference types, and unprintable tagged
values SHALL be errors. Lazy seq results SHALL be fully realized before the
check. The check SHALL run identically in interpreted and AOT modes.

#### Scenario: opaque value rejected with position
- **WHEN** `(comptime (fn [x] x))` is compiled or evaluated
- **THEN** a compile error is produced carrying the source file, line, and column of the comptime form and stating the result class is not embeddable

#### Scenario: lazy result realized then embedded
- **WHEN** `(comptime (map inc (range 3)))` is compiled
- **THEN** the realized list `(1 2 3)` is embedded

### Requirement: comptime-assert fails the build on false
The system SHALL provide `(comptime-assert <pred> <msg>)` which evaluates
`<pred>` at compile time and SHALL fail the build (AOT) or throw (interpreted)
with `<msg>` and source position when the result is falsey, and SHALL emit
nothing into the artifact when truthy.

#### Scenario: failing assert stops the build
- **WHEN** `(comptime-assert (= 1 2) "broken invariant")` is AOT-compiled
- **THEN** the build fails with a positioned error containing "broken invariant" and no binary is produced

### Requirement: embed-file embeds file contents at build time
The system SHALL provide `(embed-file "path")` returning the file's contents
as a string constant (or bytes via `(embed-file "path" :bytes)`) resolved at
compile time relative to the source file; a missing file SHALL be a
positioned compile error.

#### Scenario: file becomes a constant
- **WHEN** `(def banner (embed-file "banner.txt"))` is AOT-compiled and the file is later deleted before running the binary
- **THEN** the binary still prints the original contents of banner.txt

### Requirement: file-reading comptime participates in build caching
The system SHALL record every file read performed during comptime evaluation
(embed-file, slurp, and pkg/lang file opens) and include those files' content
hashes in the namespace's build-cache key, so changing an embedded input
invalidates the cached build; `--no-comptime-cache` SHALL force
re-evaluation of all comptime forms.

#### Scenario: editing an embedded file triggers rebuild
- **WHEN** a project embedding banner.txt is built twice with banner.txt modified between builds
- **THEN** the second build re-evaluates the embed and the new binary carries the new contents
