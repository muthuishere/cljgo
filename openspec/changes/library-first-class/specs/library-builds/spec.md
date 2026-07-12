## ADDED Requirements

### Requirement: exported surface is annotation-driven
The system SHALL determine a project's exported library surface as the union
of vars carrying `^:export` metadata and vars listed in the project file's
exports map; annotated-but-missing vars SHALL be a positioned compile error,
and unannotated vars SHALL emit as unexported Go identifiers carrying no
stability guarantee.

#### Scenario: export annotation controls Go visibility
- **WHEN** a namespace defines `(defn ^:export parse-config [s] ...)` and `(defn helper [x] ...)` and is built with `--lib`
- **THEN** the emitted package exports `ParseConfig` and emits `helper` as an unexported identifier

#### Scenario: missing export target errors
- **WHEN** the project exports map lists a symbol that no namespace defines
- **THEN** the build fails with a positioned error naming the symbol

### Requirement: --lib emits a go-gettable Go module
The system SHALL, on `cljgo build --lib`, emit a self-contained Go module —
deterministic output, module path from the project file (required), go.mod
pinning the exact cljgo runtime version, generated-code headers, doc comments
from docstrings, and real Go signatures from type hints (boxed `any`
otherwise) — such that a plain Go program can `go get` and call it with no
cljgo toolchain.

#### Scenario: Go consumes a Clojure-written library
- **WHEN** a hinted exported fn is built with `--lib`, the output is pushed to a module path, and a plain Go program imports and calls it
- **THEN** the Go program compiles with `go build` alone and the call returns the same value the fn returns under cljgo

#### Scenario: regeneration is deterministic
- **WHEN** `--lib` runs twice on identical input
- **THEN** the emitted bytes are identical

### Requirement: exported munging is a stable public contract
The system SHALL munge exported vars to Go identifiers by a single documented
deterministic scheme (kebab-to-camel with uppercase first rune atop the
JVM-compatible character munges), SHALL emit a provenance comment mapping
each exported identifier to its Clojure var, and SHALL fail the build naming
both vars when two exports collide after munging.

#### Scenario: collision is a build error
- **WHEN** `^:export parse-config` and `^:export parseConfig` exist in one namespace
- **THEN** the build fails with an error naming both source symbols and the colliding Go identifier

### Requirement: C library builds with a usable header
The system SHALL, on `cljgo build --c-shared` or `--c-archive`, produce a
.so/.a via Go's buildmodes plus a C header covering every exported var whose
hinted signature is C-expressible, a runtime header declaring an idempotent
CljgoInit, and SHALL warn (not fail) for exported vars excluded from the C
surface, naming the var and the non-expressible type.

#### Scenario: C program calls Clojure code
- **WHEN** an exported fn hinted `long -> long` is built with `--c-shared` and a C program calls CljgoInit then the fn
- **THEN** the C program links against the .so using the generated header and receives the correct value

#### Scenario: non-expressible export degrades to a warning
- **WHEN** an exported fn returning a persistent map is built with `--c-shared`
- **THEN** the build succeeds, the fn is absent from the C header, and a warning names the fn and the type
