package deps

// The PER-NAMESPACE purity gate (ADR 0054 decision 4, confirmed on real data
// by spike s50 finding 3: hiccup ships 8 pure + 2 Java namespaces in ONE jar,
// core.match 5 + 5). A whole-library gate is wrong in both directions — it
// would reject hiccup's usable eight and wave through a "mostly pure"
// library's Java namespace. The namespace is the only honest granularity.
//
// The split is: CLASSIFY AT RESOLVE, FAIL AT REQUIRE.
//   - at resolve, every namespace in the extracted tree is classified once and
//     the verdict recorded in build.lock.edn, so the purity report is
//     readable fully OFFLINE, before a byte is fetched;
//   - at require, the gate is a cheap index lookup that raises I4002 (or
//     R1012) naming the namespace, the coordinate and the offending form.
// Failing at resolve would be exactly the whole-library gate s50 rules out.
//
// Classification runs on the READER'S OUTPUT, after reader conditionals are
// resolved for :cljgo. That is strictly stronger than the balanced-paren text
// scan s50 asked for — it cannot be fooled by a `#?(` inside a string, a char
// literal, a `;` comment or a `#_` discard — and it is what makes a library
// like medley honestly consumable rather than heuristically so.

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/muthuishere/cljgo/pkg/javadetect"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// NSVerdict is one Maven-origin namespace's load verdict.
type NSVerdict struct {
	NS     string // "hiccup.compiler"
	File   string // absolute path in the extracted tree
	Coord  Coord
	Reason string // "" when loadable; otherwise the human detail
	Code   string // "" | "I4002" (real Java interop) | "R1012" (starved .cljc) | "G5017" (cljgo could not read it)
	Line   int
	Column int

	// Elided records that this file has at least one TOP-LEVEL form cljgo
	// gets nothing from (a `#?(:cljs …)` helper, say) while the rest of the
	// file loads normally. It is not a failure — the JVM elides those too —
	// but it is REPORTED, because "loadable" and "identical to the JVM's
	// namespace" are different claims and the resolve report must not blur
	// them.
	Elided bool
}

// Loadable reports whether the namespace can be required on cljgo.
func (v NSVerdict) Loadable() bool { return v.Code == "" }

// nsPurity is the per-coordinate classification result recorded in the lock.
type nsPurity struct {
	Pure []string          // usable namespace names, sorted
	Java map[string]string // ns -> "file:line detail"
}

// classifyTree classifies every Clojure source file under dir (a Maven-origin
// extracted tree) and returns both the lock-shaped summary and the per-file
// verdicts the require-time gate looks up.
func classifyTree(dir string, c Coord) (*nsPurity, []NSVerdict, error) {
	sum := &nsPurity{Java: map[string]string{}}
	var verdicts []NSVerdict

	var files []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".clj", ".cljc":
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(files)

	for _, p := range files {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil, nil, err
		}
		// Extraction already drops these; a VENDORED tree was not extracted
		// by us, so the check lives here too.
		if isMvnBuildScript(rel) {
			continue
		}
		ns := nsNameFor(rel)
		v := classifyFile(p, ns, c)
		verdicts = append(verdicts, v)
		if v.Loadable() {
			sum.Pure = append(sum.Pure, ns)
		} else {
			sum.Java[ns] = v.Reason
		}
	}
	// A .cljc and a .clj for the same namespace both map to one name; keep the
	// list unique and sorted so the lock is byte-deterministic.
	sum.Pure = uniqSorted(sum.Pure)
	return sum, verdicts, nil
}

// classifyFile reads one Maven-origin file with the starved-conditional check
// ON (ADR 0095 §4.1) and runs the consume-side certain-Java classifier over
// its forms.
func classifyFile(path, ns string, c Coord) NSVerdict {
	v := NSVerdict{NS: ns, File: path, Coord: c}

	f, err := os.Open(path)
	if err != nil {
		v.Code, v.Reason = "G5017", "cannot be read: "+err.Error()
		return v
	}
	defer f.Close()

	// The resolver is NOT optional. Without it every ::auto-resolved-keyword
	// is a read error, and mapping read errors to I4002 then asserted that a
	// 100%-pure library (com.stuartsierra/dependency, medley, hiccup.compiler)
	// "requires Java interop" — a FALSE statement about someone else's code,
	// which the honesty bar forbids outright. classifyResolver is a static,
	// evaluator-free resolver: it is enough to give ::kw and ::alias/kw a
	// namespace, which is all classification needs.
	rd := reader.New(bufio.NewReader(f),
		reader.WithFilename(path),
		reader.WithResolver(classifyResolver{ns: ns}),
		reader.WithStarvedCondError(),
		reader.WithStarvedCollError())
	forms, err := rd.ReadAll()
	if err != nil {
		if strings.Contains(err.Error(), "would change the shape of the enclosing") {
			// A shape-breaking elision is NEVER recoverable: the surviving
			// forms are structurally wrong, and calling the namespace usable
			// would hand the user a compiler error naming a library they did
			// not write.
			v.Code, v.Reason = "R1012", err.Error()
			if re, ok := err.(*reader.Error); ok {
				v.Line, v.Column = re.Pos.Line, re.Pos.Col
			}
			return v
		}
		if !strings.Contains(err.Error(), "supplies no branch for this platform") {
			// Any read error that is NOT a starved conditional is a
			// read/parse problem and gets its own code (G5017). It is
			// emphatically NOT a Java-interop verdict: cljgo failing to parse
			// a file says something about cljgo, not about whether the
			// library uses Java.
			v.Code, v.Reason = "G5017", err.Error()
			if re, ok := err.(*reader.Error); ok {
				v.Line, v.Column = re.Pos.Line, re.Pos.Col
			}
			return v
		}
		// A STARVED top-level reader conditional. The trap s50 finding 4
		// names is a .cljc whose REAL BODY is :clj-only: eliding it the way
		// the JVM does would install a namespace with no vars and blame the
		// caller later. But eliding is also what the JVM correctly does for a
		// single `#?(:cljs …)` helper, and real tools.cli 1.1.230 has exactly
		// one of those at cli.cljc:74 — treating that as fatal reported a
		// FULLY CONSUMABLE library as unusable.
		//
		// So the question is not "is there a starved conditional" but "is
		// anything LEFT". Re-read with JVM elision and count the surviving
		// top-level forms that are not the ns declaration.
		starveErr := err
		forms, err = readElided(path, ns)
		v.Elided = true
		if err != nil {
			v.Code, v.Reason = "G5017", err.Error()
			if strings.Contains(err.Error(), "would change the shape of the enclosing") {
				v.Code = "R1012"
			}
			if re, ok := err.(*reader.Error); ok {
				v.Line, v.Column = re.Pos.Line, re.Pos.Col
			}
			return v
		}
		if bodyForms(forms) == 0 {
			v.Code = "R1012"
			v.Reason = starveErr.Error()
			if re, ok := starveErr.(*reader.Error); ok {
				v.Line, v.Column = re.Pos.Line, re.Pos.Col
			}
			return v
		}
	}

	if hits := javadetect.ConsumeJava(forms); len(hits) > 0 {
		h := hits[0]
		v.Code = "I4002"
		v.Line = h.Line
		v.Reason = h.Detail
	}
	return v
}

// readElided re-reads a file with the JVM's ordinary reader-conditional
// elision (no starved-conditional error), i.e. exactly what Clojure itself
// would read on a platform none of the starved branches name.
func readElided(path, ns string) (forms []any, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// The COLLECTION check stays on: a top-level starve is safely elided, a
	// shape-breaking one never is.
	rd := reader.New(bufio.NewReader(f),
		reader.WithFilename(path),
		reader.WithResolver(classifyResolver{ns: ns}),
		reader.WithStarvedCollError())
	forms, err = rd.ReadAll()
	if err != nil {
		return nil, err
	}
	return forms, nil
}

// bodyForms counts the top-level forms that are not the namespace
// declaration. Zero means cljgo would install a namespace with no vars — the
// exact trap s50 finding 4 names, and the only case where a starved
// conditional is fatal.
func bodyForms(forms []any) int {
	n := 0
	for _, f := range forms {
		if isNSForm(f) {
			continue
		}
		n++
	}
	return n
}

// isNSForm reports whether a top-level form is (ns …) or (in-ns …).
func isNSForm(f any) bool {
	seq := lang.Seq(f)
	if seq == nil {
		return false
	}
	sym, ok := seq.First().(*lang.Symbol)
	if !ok {
		return false
	}
	switch sym.Name() {
	case "ns", "in-ns":
		return true
	}
	return false
}

// classifyResolver is the reader.Resolver classification reads with: static,
// evaluator-free, and total. CurrentNS is the namespace the file declares by
// its path, so ::kw resolves; ResolveAlias echoes the alias back, so
// ::alias/kw resolves without needing the file's requires to have been
// processed (classification cares about FORM SHAPES, never about keyword
// identity). ResolveVar/ResolveType return nil — a syntax-quoted symbol
// simply qualifies into the current namespace, which is also shape-neutral.
type classifyResolver struct{ ns string }

func (r classifyResolver) CurrentNS() *lang.Symbol { return lang.NewSymbol(r.ns) }

func (r classifyResolver) ResolveAlias(sym *lang.Symbol) *lang.Symbol { return sym }

func (r classifyResolver) ResolveVar(sym *lang.Symbol) *lang.Symbol { return nil }

func (r classifyResolver) ResolveType(sym *lang.Symbol) *lang.Symbol { return nil }

// mvnBuildScripts are jar-root files that are BUILD SCRIPTS, not library
// namespaces. Leiningen's project.clj is routinely packaged at the jar root
// (hiccup ships one); counting it made the resolve report claim "7 namespaces
// usable" for a library with 6, and put a bogus "project" in :mvn/pure. This
// is the same false-positive class as the META-INF/…/project.clj case s50 hit,
// which extraction already drops — this is its sibling at the root.
var mvnBuildScripts = map[string]bool{
	"project.clj": true, "build.clj": true, "build.boot": true,
	"boot.clj": true, "shadow-cljs.clj": true,
}

// isMvnBuildScript reports whether an extracted-tree relative path is a
// root-level build script. Only the ROOT is special: a real namespace named
// `project` would live at project.clj too, but no published library has one,
// whereas nearly every Leiningen jar has the build script.
func isMvnBuildScript(rel string) bool {
	rel = filepath.ToSlash(rel)
	if strings.Contains(rel, "/") {
		return false
	}
	return mvnBuildScripts[strings.ToLower(rel)]
}

// nsNameFor maps an extracted-tree relative path to its namespace name,
// undoing the JVM munging: hiccup/util.clj -> hiccup.util,
// clojure/tools/cli.cljc -> clojure.tools.cli, my_app/core.clj -> my-app.core.
func nsNameFor(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))
	rel = strings.ReplaceAll(rel, "_", "-")
	return strings.ReplaceAll(rel, "/", ".")
}

func uniqSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// ClassifyMavenTree classifies an already-extracted Maven source tree and
// returns the per-namespace verdicts, for callers outside the resolve pass
// (a purity report over a warm cache, or an integration test).
func ClassifyMavenTree(dir string, c Coord) ([]NSVerdict, error) {
	_, verdicts, err := classifyTree(dir, c)
	return verdicts, err
}

// ---- the require-time index ----------------------------------------------

// A process-scoped index of Maven-origin source files, published by the
// resolver alongside SetResolvedRoots and read by the loader's require hook.
// One handle, set once at bootstrap, so the interpreter leg and the AOT leg
// hit the SAME gate — parity by construction. The AOT leg is the important
// one: the emitter discovers namespaces by evaluating requires (ADR 0042), so
// a Java namespace can never be silently emitted.
var (
	mvnIndexMu sync.RWMutex
	mvnIndex   map[string]NSVerdict // absolute file path -> verdict
	mvnUsable  map[string][]string  // coordinate key -> usable namespaces
)

// SetMavenIndex publishes the Maven-origin file verdicts. Passing nil clears
// it (a project with no Maven deps pays nothing).
func SetMavenIndex(verdicts []NSVerdict) {
	mvnIndexMu.Lock()
	defer mvnIndexMu.Unlock()
	mvnIndex = nil
	mvnUsable = nil
	if len(verdicts) == 0 {
		return
	}
	mvnIndex = make(map[string]NSVerdict, len(verdicts))
	mvnUsable = map[string][]string{}
	for _, v := range verdicts {
		abs := v.File
		if a, err := filepath.Abs(abs); err == nil {
			abs = a
		}
		v.File = abs
		mvnIndex[abs] = v
		if v.Loadable() {
			mvnUsable[v.Coord.Key()] = append(mvnUsable[v.Coord.Key()], v.NS)
		}
	}
}

// CheckMavenLoadable is the REQUIRE-TIME gate. Given a source path the load
// path resolved to, it returns nil unless that file came out of a Maven
// dependency and cannot load on cljgo, in which case it returns the coded
// diagnostic (I4002 or R1012). A path that is not Maven-origin is never
// touched — project code and git deps are completely unaffected.
func CheckMavenLoadable(path string) error {
	mvnIndexMu.RLock()
	idx, usable := mvnIndex, mvnUsable
	mvnIndexMu.RUnlock()
	if idx == nil {
		return nil
	}
	abs := path
	if a, err := filepath.Abs(path); err == nil {
		abs = a
	}
	v, ok := idx[abs]
	if !ok || v.Loadable() {
		return nil
	}

	if v.Code == "R1012" {
		// The reader already produced the located, expected-vs-found message.
		return codedf("R1012", "namespace %s cannot load on cljgo: %s", v.NS, v.Reason).
			note("it came from the maven dependency " + v.Coord.String()).
			withFix(otherUsableHint(usable, v))
	}
	if v.Code == "G5017" {
		// A parse failure, NOT a Java verdict. The message says exactly that,
		// because claiming "requires Java interop" about a file cljgo merely
		// failed to read is an assertion about someone else's library that
		// cljgo cannot support.
		e := codedf("G5017", "namespace %s cannot be read by cljgo's reader — %s", v.NS, v.Reason)
		if v.Line > 0 {
			e = e.at(v.File, v.Line, v.Column)
		}
		return e.note("it came from the maven dependency " + v.Coord.String()).
			note("this is a cljgo reader limitation, not a statement about the library").
			withFix("report it with the file and line above — the library may be perfectly pure Clojure")
	}
	e := codedf("I4002", "namespace %s requires Java interop and cannot load on cljgo — %s", v.NS, v.Reason)
	if v.Line > 0 {
		e = e.at(v.File, v.Line, 1)
	}
	e.note("it came from the maven dependency " + v.Coord.String())
	return e.withFix(otherUsableHint(usable, v))
}

// otherUsableHint is the per-namespace granularity made visible: the library
// is installed and its pure namespaces are usable; only THIS one failed.
func otherUsableHint(usable map[string][]string, v NSVerdict) string {
	n := len(usable[v.Coord.Key()])
	if n == 0 {
		// Deliberately NOT "it is a JVM-only library". Zero usable namespaces
		// is a fact about what cljgo could load; "JVM-only" is a claim about
		// the library, and cljgo has been wrong about that before (a single
		// missing reader option once made pure-Clojure libraries read as
		// JVM-only). State the measurement, not the verdict.
		return "no namespace in " + v.Coord.String() + " loaded on cljgo — see the resolve report for why, per namespace"
	}
	return plural(n) + " in " + v.Coord.String() + " are usable — see the resolve report, or :mvn/namespaces in build.lock.edn"
}

func plural(n int) string {
	if n == 1 {
		return "1 other namespace"
	}
	return itoa(n) + " other namespaces"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// MavenUsableNamespaces returns the usable namespaces per coordinate key, for
// the resolve report.
func MavenUsableNamespaces() map[string][]string {
	mvnIndexMu.RLock()
	defer mvnIndexMu.RUnlock()
	out := map[string][]string{}
	for k, v := range mvnUsable {
		out[k] = append([]string(nil), v...)
	}
	return out
}
