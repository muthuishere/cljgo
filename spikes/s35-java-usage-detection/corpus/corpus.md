# S35 labeled corpus (30 forms)

Truth labels are the **JVM meaning**: `java-interop` snippets were run on
`clojure` 1.12.5.1645 and confirmed to be genuine, working JVM interop
(`results/oracle-jvm.txt`); `go-interop` uses cljgo-only `require-go` (no
JVM meaning by construction); `pure` runs identically on the JVM. The
machine-readable corpus lives in `proto/main.go` (`var corpus`); this file
is the human index. cljgo's per-snippet behavior is in
`results/cljgo-behavior.txt`.

## pure (10)
| snippet | note |
|---|---|
| `(reduce + (map inc [1 2 3]))` | arithmetic |
| `(clojure.string/upper-case "hi")` | clojure.string is a pure ns |
| `(let [m {:a 1}] (get m :a))` | map access |
| `(defn f [x] (* x x))` | defn |
| `(filter even? (range 10))` | seq lib |
| `(str "a" "b" "c")` | str |
| `(instance? String "x")` | **TRAP** — class-syntax, not a value (ADR 0026) |
| `(try 1 (catch Exception e 2))` | **TRAP** — catch class name is host-neutral |
| `(def x String)` | **TRAP** — bare ClassRef value (ADR 0036) |
| `(pr-str java.util.UUID)` | **TRAP** — `java.*` ClassRef value, still pure |

## go-interop (5) — impure by ADR 0052 §6, but NOT Java
| snippet | note |
|---|---|
| `(require-go '[strings :as strs]) (strs/ToUpper "hi")` | Go ns call |
| `(require-go '[strconv :as sc]) (sc/Itoa 42)` | Go ns call |
| `(require-go '[strings]) (def r (strings/NewReplacer "a" "1")) (.Replace r "abc")` | **Go dot-method** — host-neutral shape |
| `(require-go '[os]) (os/Getpid)` | Go ns call |
| `(require-go '[math :as m]) (m/Sqrt 2.0)` | Go Math analog — overlaps JVM `Math` |

## java-interop (15) — real JVM, unsupported on cljgo
| snippet | note | cljgo error stage |
|---|---|---|
| `(java.util.UUID/randomUUID)` | `java.*` static call | analysis |
| `(java.time.Instant/now)` | `java.*` static call | analysis |
| `(System/currentTimeMillis)` | bare JVM class `System` | analysis |
| `(Math/sqrt 2)` | bare JVM class `Math` (overlap trap) | analysis |
| `(Thread/sleep 1)` | bare JVM class `Thread` | analysis |
| `(Integer/parseInt "42")` | bare JVM class `Integer` | analysis |
| `(String/valueOf 5)` | bare JVM class `String` (call ns) | analysis |
| `(new java.io.File "x")` | `new` + `java.*` class | analysis |
| `(import '[java.util Date]) (Date.)` | `import` + ctor | analysis |
| `(clojure.java.io/file "x")` | `clojure.java.*` ns | analysis |
| `(.toUpperCase "hello")` | **AMBIG** — Java dot-method, host-neutral shape | RUNTIME |
| `(.getBytes "x")` | **AMBIG** — Java dot-method | RUNTIME |
| `(.length "hello")` | **AMBIG** — Java dot-method | RUNTIME |
| `(defn up [s] (.toUpperCase s))` | **AMBIG** — Java dot-method, uncalled | **never (silent ok)** |

The four `AMBIG` dot-forms are the irreducible residual (VERDICT §2): no
analysis-time signal separates them from a valid Go method call, and the
last one produces no error at all today.
