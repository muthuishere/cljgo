## ADDED Requirements

### Requirement: reader features are exactly :cljgo and :default
The reader SHALL answer reader-conditional features `:cljgo` and
`:default` only — never `:clj` or any other dialect's feature — with
first-match-wins selection and elision (selecting and splicing alike)
when no branch matches, exactly as JVM Clojure elides under
`{:read-cond :allow}` with a non-matching feature set.

#### Scenario: no-match selecting conditional elides
- **WHEN** `[1 #?(:cljs 2) 3]` is read
- **THEN** the result is `[1 3]`

### Requirement: well-known class names resolve to interned class refs
A fixed, fail-closed table of well-known JVM class names SHALL resolve —
only after every normal var-resolution path has missed — to interned,
opaque ClassRef values with identity equality, canonicalized so simple
and fully qualified spellings (`String`, `java.lang.String`) are the
same value, printing as the canonical name. No type inheritance SHALL
be fabricated around them.

#### Scenario: class ref as a hierarchy tag
- **WHEN** `(derive String ::object)` is evaluated
- **THEN** `(isa? String ::object)` is true and `(parents String)` is
  `#{::object}`; after `(underive String ::object)`, `(parents String)`
  is nil

#### Scenario: classes are rejected where the JVM rejects them
- **WHEN** `(derive ::tag String)` or `(descendants Object)` is evaluated
- **THEN** each throws, matching JVM Clojure 1.12.5 (parent must be
  Named; "Can't get descendants of classes")

#### Scenario: user definitions always win
- **WHEN** a namespace defines or binds a name in the table
- **THEN** that definition resolves, never the class ref

### Requirement: class? recognizes cljgo's class analogs
`clojure.core/class?` SHALL return true for ClassRef values and for
deftype/defrecord type markers, false for everything else.

#### Scenario: class? over representative values
- **WHEN** `(class? String)`, `(class? SomeRecordType)`, `(class? 42)`
  are evaluated
- **THEN** the results are true, true, false
