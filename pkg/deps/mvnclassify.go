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
	"github.com/muthuishere/cljgo/pkg/reader"
)

// NSVerdict is one Maven-origin namespace's load verdict.
type NSVerdict struct {
	NS     string // "hiccup.compiler"
	File   string // absolute path in the extracted tree
	Coord  Coord
	Reason string // "" when loadable; otherwise the human detail
	Code   string // "" | "I4002" | "R1012"
	Line   int
	Column int
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
			if isJarNoise(dir, p) {
				return nil
			}
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
		v.Code, v.Reason = "I4002", "cannot be read: "+err.Error()
		return v
	}
	defer f.Close()

	rd := reader.New(bufio.NewReader(f),
		reader.WithFilename(path),
		reader.WithStarvedCondError())
	forms, err := rd.ReadAll()
	if err != nil {
		// A starved reader conditional is R1012 and is fatal for this file: a
		// .cljc whose real body is :clj-only would otherwise install a
		// namespace with no vars and blame the caller later.
		v.Code = "R1012"
		if !strings.Contains(err.Error(), "supplies no branch for this platform") {
			// Any other read error is still a hard, loud failure at require —
			// never a silently empty namespace — but it keeps its own code.
			v.Code = "I4002"
		}
		v.Reason = err.Error()
		if re, ok := err.(*reader.Error); ok {
			v.Line, v.Column = re.Pos.Line, re.Pos.Col
		}
		return v
	}

	if hits := javadetect.ConsumeJava(forms); len(hits) > 0 {
		h := hits[0]
		v.Code = "I4002"
		v.Line = h.Line
		v.Reason = h.Detail
	}
	return v
}

// nsNameFor maps an extracted-tree relative path to its namespace name,
// undoing the JVM munging: hiccup/util.clj -> hiccup.util,
// clojure/tools/cli.cljc -> clojure.tools.cli, my_app/core.clj -> my-app.core.
// isJarNoise reports whether a root-level .clj is a BUILD/tooling file rather
// than a namespace. Clojars jars very commonly ship project.clj, build.boot or
// data_readers.clj at the jar root, and none of them is a requirable namespace
// — project.clj is Leiningen's build script, data_readers.clj is a reader-tag
// map the runtime consumes directly.
//
// Counting them inflated the usable-namespace figure the resolve report and
// the I4002 fix line quote: a one-namespace library shipping project.clj and
// data_readers.clj reported "3 namespace(s) usable". The GATE was always
// correct (it keys on file path, not on this count) — the honest NUMBER was
// not, and a number a user cannot reconcile with the library's own docs is
// worse than no number. Found by the adversarial verifier, 2026-07-30.
//
// Root-level only: a real namespace can legitimately be called
// my.lib.project, and that lives at my/lib/project.clj, not at the root.
func isJarNoise(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if strings.Contains(rel, "/") {
		return false // nested: a genuine namespace path
	}
	switch strings.ToLower(rel) {
	case "project.clj", "build.boot", "data_readers.clj", "data_readers.cljc":
		return true
	}
	return false
}

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
			withFix(otherUsableHint(usable, v))
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
		return "no namespace in " + v.Coord.String() + " is usable on cljgo — it is a JVM-only library"
	}
	return plural(n) + " in " + v.Coord.String() + " — see the resolve report, or :mvn/namespaces in build.lock.edn"
}

// plural carries its own verb: "1 other namespace IS usable" but "3 other
// namespaces ARE usable". The verb used to sit in the caller, which read
// "1 other namespace … are usable".
func plural(n int) string {
	if n == 1 {
		return "1 other namespace is usable"
	}
	return itoa(n) + " other namespaces are usable"
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
