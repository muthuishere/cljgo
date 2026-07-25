# resource-generator Specification (delta)

## ADDED Requirements

### Requirement: `cljgo generate resource` scaffolds a CRUD resource into a bri web app

`cljgo generate resource <Name> <field:type>...` (alias `cljgo g`) MUST run
from a bri web app root (an existing `src/app/main.cljg`) and, from the
singular `Name` and zero-or-more `name:type` fields, produce a complete
authenticated CRUD resource. Running outside such a directory, with an
unknown field type, with a malformed field token, or with a non-identifier
name MUST fail with a named error and MUST NOT leave a partially generated
tree behind.

#### Scenario: Generating a resource in a web app succeeds
- **WHEN** `cljgo generate resource Note title:string body:text` runs in a directory containing `src/app/main.cljg`
- **THEN** the command exits 0 and reports each created file and the splice into `src/app/main.cljg`

#### Scenario: Generating outside a web app is a clean error
- **WHEN** `cljgo generate resource Note title:string` runs where no `src/app/main.cljg` exists
- **THEN** the command exits non-zero with a message pointing at `cljgo new --template web`, and writes no resource files

#### Scenario: An unknown field type is rejected
- **WHEN** a field uses a type outside `string text int bool uuid timestamp references` (e.g. `qty:decimal`)
- **THEN** the command exits non-zero, names the offending token, and lists the known types

### Requirement: The field-type grammar maps each field to a column and a coercion

Each `name:type` field MUST map to a SQLite column and a Clojure coercion:
`string`/`text`/`uuid`/`timestamp` → `TEXT` (coerced with `str`); `int` →
`INTEGER` (coerced number-or-`parse-long`); `bool` → `INTEGER` (coerced
truthy); `references` → a `<name>_id INTEGER` column WITH an index, coerced
number-or-`parse-long`. Every table MUST also get an implicit `id INTEGER
PRIMARY KEY AUTOINCREMENT`, and the member routes MUST coerce the `{id}`
path param via `http/param! :int`.

#### Scenario: A references field becomes an indexed foreign-key column
- **WHEN** `cljgo generate resource Post author:references` runs
- **THEN** the migration declares `author_id INTEGER NOT NULL` and a `CREATE INDEX ... ON posts (author_id)`

#### Scenario: Numeric and boolean fields get typed coercion
- **WHEN** a resource declares `views:int` and `done:bool`
- **THEN** the generated resource ns coerces incoming JSON `views` through a long coercion and `done` through a boolean coercion before the write

### Requirement: The generator creates the resource slice and splices routes at markers

The command MUST create a timestamped migration
(`db/migrations/<ts>_create_<plural>.sql`), a resource ns
(`src/app/<plural>.cljg`) whose model section is the only caller of bri.db
and whose handlers use the bri.http surface, an in-process test
(`test/app/<plural>_test.cljg`), and — once, if absent — an `app.db`
datasource ns. It MUST edit `src/app/main.cljg` by inserting the resource's
`:require` entry and its `routes` value ABOVE the `;; cljgo:resource-requires`
and `;; cljgo:resource-routes` comment markers, without parsing
s-expressions. Every emitted `.cljg` MUST be valid source, and the spliced
`main.cljg` MUST remain valid source.

#### Scenario: main.cljg is spliced at the markers
- **WHEN** a resource `Note` is generated into the stock `web` template
- **THEN** `src/app/main.cljg` gains `[app.notes :as notes]` in its `:require` and `notes/routes` inside `(http/routes …)`, both markers remain for the next resource, and the file still reads as valid Clojure

#### Scenario: A main.cljg without the markers is not edited blindly
- **WHEN** `src/app/main.cljg` has had a marker removed
- **THEN** the command exits non-zero, names the missing marker and the lines to add, and does not edit the file

### Requirement: Generation is idempotent and never clobbers user edits

Re-running for an already-generated resource MUST refuse to overwrite
`src/app/<plural>.cljg` unless `--force` is given. The `main.cljg` splice
MUST be idempotent — an already-present require or `routes` value MUST NOT
be duplicated on a re-run or a `--force` run. The shared `app.db` ns MUST
be created only when it does not already exist.

#### Scenario: Re-running without --force refuses to overwrite
- **WHEN** `cljgo generate resource Note ...` runs and `src/app/notes.cljg` already exists
- **THEN** the command exits non-zero and leaves the existing file untouched

#### Scenario: --force re-run does not duplicate the splice
- **WHEN** `cljgo generate resource --force Note ...` re-generates an already-wired resource
- **THEN** `src/app/main.cljg` still contains exactly one `notes/routes` entry and one `app.notes` require

### Requirement: Naming and pluralization are derived from the resource name

The given `Name` is the singular; the ns, table, route base, and collection
key MUST use its plural (minimal ruleset: consonant+`y`→`ies`,
`s/x/z/ch/sh`→`es`, else `+s`). The resource ns MUST be `app.<plural>`, the
table and JSON collection key `<plural>`, the route base `/api/<plural>`,
and the reverse-route names `:<plural>` (collection) and `:<singular>`
(member).

#### Scenario: The plural drives the generated names
- **WHEN** `cljgo generate resource City name:string` runs
- **THEN** the ns is `app.cities`, the table `cities`, the routes are under `/api/cities`, and the member route is named `:city`
