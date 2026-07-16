// Package core embeds the bootstrap clojure.core source (design/00 §6,
// M1). pkg/eval loads Source into the clojure.core namespace at startup,
// after the Go builtins and the hand-built defmacro are interned; the
// embed lives here (its own top-level package) because go:embed cannot
// reach outside a package directory, and pkg/eval imports it.
package core

import _ "embed"

// Source is the contents of core.clj.
//
//go:embed core.clj
var Source string

// StringSource is the contents of string.cljg — the clojure.string
// namespace, built on the regex core fns and the private `-str-*` host
// primitives (pkg/eval/string_builtins.go). pkg/eval loads it into the
// clojure.string namespace after clojure.core is up (loadClojureString).
// The loader accepts the .cljg extension per ADR 0017.
//
//go:embed string.cljg
var StringSource string

// TestSource is the contents of test.cljg — the interpreted clojure.test
// slice (ADR 0012 / openspec testing-first-class). pkg/eval loads it into
// the clojure.test namespace after clojure.core is up (loadClojureTest).
// The loader accepts the .cljg extension per ADR 0017.
//
//go:embed test.cljg
var TestSource string

// ProtocolsSource is the contents of protocols.cljg — the Clojure-level
// macro surface for the polymorphism layer (defprotocol / deftype /
// defrecord / extend-type / extend-protocol). pkg/eval loads it into
// clojure.core after core.clj is up (loadProtocols), so the macros are
// referred into user like the rest of core. The runtime dispatch/instance
// builtins they expand onto live in pkg/eval/protocols.go.
//
//go:embed protocols.cljg
var ProtocolsSource string

// BuildSource is the contents of build.cljg — the interpreted Zig-style
// build runtime (ADR 0021 / design/08 §1). pkg/eval loads it into the
// cljgo.build namespace after clojure.core is up (loadBuild). `cljgo build`
// (no file arg) evaluates a project's build.cljgo against this library and
// reads back the build plan. The loader accepts the .cljg extension per
// ADR 0017.
//
//go:embed build.cljg
var BuildSource string

// PredicatesSource is the contents of predicates.cljg — the Batch 1
// "cheap breadth" clojure.core fns that compose over the Go predicate/
// coercion builtins (ADR 0022, design/08 §5): the simple/qualified ident
// family, pos-int?/neg-int?/nat-int?, ffirst/nfirst/fnext/last/butlast/
// drop-last/take-last, and not=. pkg/eval loads it into clojure.core right
// after core.clj (loadPredicates), so its publics are referred into user
// like the rest of core. The loader accepts the .cljg extension per ADR 0017.
//
//go:embed predicates.cljg
var PredicatesSource string

// PortabilitySource is the contents of clojure_test_portability.cljg — the
// cljgo shim for the jank clojure-test-suite's clojure.core-test.portability
// namespace (when-var-exists + big-int?/lazy-seq?, ADR 0022). pkg/eval loads
// it into that namespace after clojure.core + clojure.test are up
// (loadClojureTestPortability), so a suite file's (require …portability…)
// finds it already interned. The loader accepts the .cljg extension per ADR 0017.
//
//go:embed clojure_test_portability.cljg
var PortabilitySource string

// NumericSource is the contents of numeric.cljg — the Clojure-level
// numeric-tower fns (random-sample, …) that ride on the Batch 2 host
// primitives (pkg/eval/numeric_builtins.go, design/08 §5). pkg/eval loads
// it into clojure.core after core.clj is up (loadNumeric), so its publics
// are referred into user like the rest of core. The loader accepts the
// .cljg extension per ADR 0017.
//
//go:embed numeric.cljg
var NumericSource string

// ReplSource is the contents of repl.cljg — the clojure.repl namespace
// (ADR 0031: `doc` + `print-doc`). pkg/eval loads it into the
// clojure.repl namespace after clojure.core is up (loadClojureRepl) and
// refers `doc` into user at boot, as JVM clojure.main's repl-requires
// does. The loader accepts the .cljg extension per ADR 0017.
//
//go:embed repl.cljg
var ReplSource string

// SetSource is the contents of set.cljg — the clojure.set namespace, a pure
// port of clojure.set onto core.clj primitives (ADR 0022 batch/harness-misc).
// pkg/eval loads it into the clojure.set namespace after clojure.core is up
// (loadClojureSet). The loader accepts the .cljg extension per ADR 0017.
//
//go:embed set.cljg
var SetSource string

// EdnSource is the contents of edn.cljg — the clojure.edn namespace
// (read-string over the -edn-read-string reader seam, ADR 0022
// batch/harness-misc). pkg/eval loads it into the clojure.edn namespace
// after clojure.core is up (loadClojureEdn). The loader accepts the .cljg
// extension per ADR 0017.
//
//go:embed edn.cljg
var EdnSource string

// TransducersSource is the contents of transducers.cljg — transduce/
// eduction/sequence/completing/partition-by/dedupe/halt-when/replace, plus
// the `into` xform arity (design/08 §5 Batch 4, ADR 0022). pkg/eval loads it
// into clojure.core after core.clj (loadTransducers), so its publics are
// referred into user like the rest of core. The loader accepts the .cljg
// extension per ADR 0017.
//
//go:embed transducers.cljg
var TransducersSource string

// HierarchiesSource is the contents of hierarchies.cljg — the global
// hierarchy family (make-hierarchy/derive/underive/ancestors/descendants/
// parents/isa?, ADR 0022 Track E, design/08 §5 batch E). pkg/eval loads it
// into clojure.core after loadNumeric (loadHierarchies), so these are
// referred into user like the rest of core. The loader accepts the .cljg
// extension per ADR 0017.
//
//go:embed hierarchies.cljg
var HierarchiesSource string
