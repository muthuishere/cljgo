package deps

// The .pom reader: a deliberately SMALL, pure-Go subset of Maven (encoding/xml
// only — no JVM, no mvn, no Aether), plus a validator that NAME-ERRORS every
// feature outside the subset instead of half-supporting it.
//
// WHAT CHANGED (2026-07-30, adversarial verification against the live
// repositories). Spike s50 finding 1 said `<parent>`, `${property}` and
// `<dependencyManagement>` must all name-error. That was RIGHT about guessing
// and WRONG about scope: EVERY org.clojure contrib artifact carries
// `<parent>org.clojure/pom.contrib</parent>`, so the refusal excluded
// tools.cli, data.json, data.csv and core.match — the entire target set this
// feature exists to consume. The refusal is now narrower and sharper:
//
//   - `<parent>` IS resolved. The parent POM is fetched like any other
//     artifact and the child inherits groupId/version defaults, <properties>,
//     <dependencyManagement> and <dependencies>. Chains are walked to a depth
//     limit; an unresolvable parent is a G5010 that says whose parent it is.
//   - `${property}` is interpolated ONLY from properties actually defined in
//     the merged (parent-then-child) property map, plus the built-in
//     project.* set. A property with no definition still name-errors — an
//     uninterpolated version is a WRONG version, and that is the part of s50
//     finding 1 that stands.
//   - `<dependencyManagement>` supplies a missing <version> when the merged
//     map actually has one; a missing <version> with no managed entry still
//     name-errors.
//   - `<profiles>` name-errors only when a profile could change the
//     DEPENDENCY GRAPH (it declares <dependencies>, <dependencyManagement> or
//     <properties>). pom.contrib carries a gpg-signing profile that touches
//     only <build>; refusing on that would again exclude every contrib
//     artifact, and a build-plugin profile provably cannot change what we
//     resolve.
//
// Each G5011 is still raised at the coordinate that needs the feature, so the
// message says which library, not "a pom failed".

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"unicode/utf8"
)

// maxParentDepth caps the <parent> chain walk. Real chains are 1 deep
// (contrib -> pom.contrib); 8 is generous and turns a cyclic or absurd chain
// into a named error rather than an unbounded fetch loop.
const maxParentDepth = 8

// pomXML is the parsed subset of a POM.
type pomXML struct {
	XMLName    xml.Name `xml:"project"`
	Parent     *pomRef  `xml:"parent"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Version    string   `xml:"version"`
	Packaging  string   `xml:"packaging"`

	Properties *pomProps `xml:"properties"`

	DependencyManagement *struct {
		Dependencies []pomDepXML `xml:"dependencies>dependency"`
	} `xml:"dependencyManagement"`

	Profiles []pomProfile `xml:"profiles>profile"`

	Dependencies []pomDepXML `xml:"dependencies>dependency"`
}

// pomProps captures <properties> as an ordered list of arbitrary elements —
// property names are user-chosen, so no struct can name them.
type pomProps struct {
	Entries []pomProp `xml:",any"`
}

type pomProp struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

// pomProfile is a <profile>, kept only far enough to decide whether it could
// change the dependency graph.
type pomProfile struct {
	ID                   string    `xml:"id"`
	Dependencies         *struct{} `xml:"dependencies"`
	DependencyManagement *struct{} `xml:"dependencyManagement"`
	Properties           *struct{} `xml:"properties"`
}

// affectsGraph reports whether this profile could change what we resolve. A
// profile that only configures <build> plugins provably cannot.
func (p pomProfile) affectsGraph() bool {
	return p.Dependencies != nil || p.DependencyManagement != nil || p.Properties != nil
}

type pomRef struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type pomDepXML struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Optional   string `xml:"optional"`
	Classifier string `xml:"classifier"`
	Type       string `xml:"type"`
	Exclusions []struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
	} `xml:"exclusions>exclusion"`
}

// pomEdge is one followed dependency edge, after scope/optional/exclusion
// filtering and the clojure-itself prune.
type pomEdge struct {
	Coord      Coord
	Exclusions []string // "group:artifact", "group:*", "*:*"
}

// skippedScopes are the scopes that never contribute to a runtime graph.
var skippedScopes = map[string]bool{
	"test": true, "provided": true, "system": true, "import": true,
}

// parsePOM parses raw .pom bytes.
func parsePOM(b []byte, c Coord) (*pomXML, error) {
	var p pomXML
	// A decoder with a CharsetReader, not xml.Unmarshal: encoding/xml refuses
	// any declared encoding other than UTF-8 unless one is supplied, and it
	// fails with "xml: encoding \"ISO-8859-1\" declared but Decoder.CharsetReader
	// is nil" — a Go implementation detail that leaked verbatim into a G5011
	// telling the user "the repository served something that is not a Maven
	// POM". It IS a Maven POM; we simply refused to read it. Real ones hit
	// this (commons-parent, ~1.4% of the poms cached on one machine — spike
	// s79), and it is unrecoverable for the user: nothing about their
	// coordinate is wrong.
	//
	// Latin-1 is decoded properly (each byte is the code point of the same
	// number); anything else is passed through as-is rather than mangled,
	// which for the ASCII-range text a POM actually contains — coordinates,
	// versions, property names — reads correctly. Only prose in <description>
	// could garble, and no resolution decision reads it.
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.CharsetReader = pomCharsetReader
	if err := dec.Decode(&p); err != nil {
		return nil, codedf("G5011", "cannot parse the POM of %s: %v", c, err).
			withFix("the repository served something that is not a Maven POM; check the coordinate, or depend via :git")
	}
	return &p, nil
}

// pomCharsetReader supplies encoding/xml with a reader for a non-UTF-8 POM.
// Latin-1 is converted correctly; every other declared encoding is passed
// through unchanged, on the grounds that failing to resolve a dependency is a
// worse outcome than possibly garbling prose in a field no resolution
// decision reads.
func pomCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "iso-8859-1", "latin1", "iso_8859-1", "windows-1252":
		return &latin1Reader{r: bufio.NewReader(input)}, nil
	default:
		return input, nil
	}
}

// latin1Reader converts ISO-8859-1 to UTF-8 one byte at a time: in Latin-1
// every byte IS the code point of the same number, so the conversion is a
// rune cast and needs no table.
type latin1Reader struct {
	r   *bufio.Reader
	buf []byte
}

func (l *latin1Reader) Read(p []byte) (int, error) {
	for len(l.buf) < len(p) {
		b, err := l.r.ReadByte()
		if err != nil {
			if len(l.buf) == 0 {
				return 0, err
			}
			break
		}
		l.buf = utf8.AppendRune(l.buf, rune(b))
	}
	n := copy(p, l.buf)
	l.buf = l.buf[n:]
	return n, nil
}

// ---- effective POM (the <parent> merge) -----------------------------------

// effPOM is the child POM after its <parent> chain has been merged in: the
// only shape pomChildren ever looks at.
type effPOM struct {
	coord     Coord  // the CHILD coordinate the merge was performed for
	packaging string // the CHILD's packaging; a parent is always <packaging>pom
	props     map[string]string
	managed   map[string]string // "group/artifact" -> raw version
	deps      []pomDepXML
	profiles  []pomProfile // graph-affecting profiles, child + inherited
	chain     []Coord      // the parent coordinates actually fetched, nearest first
}

// parentFetcher fetches and parses one parent POM. resolveMvn supplies the
// real (cache-then-network) one; tests supply the same thing pointed at the
// repository double.
type parentFetcher func(Coord) (*pomXML, error)

// effectivePOM walks p's <parent> chain and merges it into one effective POM.
// Inheritance follows Maven for the parts that can change a dependency graph:
// groupId/version default down, and <properties>, <dependencyManagement> and
// <dependencies> merge with the CHILD winning on a conflict.
func effectivePOM(p *pomXML, c Coord, fetch parentFetcher) (*effPOM, error) {
	// Collect the chain child-first.
	chain := []*pomXML{p}
	var coords []Coord
	cur, curCoord := p, c
	for cur.Parent != nil && (strings.TrimSpace(cur.Parent.GroupID) != "" || strings.TrimSpace(cur.Parent.ArtifactID) != "") {
		if len(coords) >= maxParentDepth {
			return nil, unsupportedPOM(c, "a <parent> chain deeper than the supported limit",
				itoa(maxParentDepth)+" levels")
		}
		pc, err := parentCoord(cur.Parent, curCoord, c)
		if err != nil {
			return nil, err
		}
		for _, seen := range coords {
			if seen == pc {
				return nil, unsupportedPOM(c, "a cyclic <parent> chain", pc.String())
			}
		}
		pp, err := fetch(pc)
		if err != nil {
			if de, ok := err.(*DiagError); ok {
				return nil, de.note("it is the <parent> POM of " + curCoord.String() +
					", which " + c.String() + " needs in order to resolve")
			}
			return nil, err
		}
		chain = append(chain, pp)
		coords = append(coords, pc)
		cur, curCoord = pp, pc
	}

	e := &effPOM{
		coord:     c,
		packaging: strings.TrimSpace(p.Packaging),
		props:     map[string]string{},
		managed:   map[string]string{},
		chain:     coords,
	}
	// Merge root-most first so the child overwrites.
	for i := len(chain) - 1; i >= 0; i-- {
		mergePOMInto(e, chain[i])
	}
	// The built-in project.* properties, resolved against the CHILD coordinate.
	for k, v := range map[string]string{
		"project.groupId": c.Group, "project.artifactId": c.Artifact, "project.version": c.Version,
		"pom.groupId": c.Group, "pom.artifactId": c.Artifact, "pom.version": c.Version,
	} {
		e.props[k] = v
	}
	return e, nil
}

// mergePOMInto folds one POM of the chain into the accumulating effective POM.
func mergePOMInto(e *effPOM, p *pomXML) {
	if p.Properties != nil {
		for _, pr := range p.Properties.Entries {
			e.props[pr.XMLName.Local] = strings.TrimSpace(pr.Value)
		}
	}
	if p.DependencyManagement != nil {
		for _, d := range p.DependencyManagement.Dependencies {
			g, a := strings.TrimSpace(d.GroupID), strings.TrimSpace(d.ArtifactID)
			if g == "" || a == "" {
				continue
			}
			if v := strings.TrimSpace(d.Version); v != "" {
				e.managed[g+"/"+a] = v
			}
		}
	}
	for _, pf := range p.Profiles {
		if pf.affectsGraph() {
			e.profiles = append(e.profiles, pf)
		}
	}
	// A dependency the child redeclares REPLACES the inherited one.
	for _, d := range p.Dependencies {
		key := strings.TrimSpace(d.GroupID) + "/" + strings.TrimSpace(d.ArtifactID)
		replaced := false
		for i, prev := range e.deps {
			if strings.TrimSpace(prev.GroupID)+"/"+strings.TrimSpace(prev.ArtifactID) == key {
				e.deps[i] = d
				replaced = true
				break
			}
		}
		if !replaced {
			e.deps = append(e.deps, d)
		}
	}
}

// parentCoord builds the parent's coordinate, defaulting nothing it must not
// guess at. A parent with no version is a name-error, not "pick the latest".
func parentCoord(ref *pomRef, child, owner Coord) (Coord, error) {
	g := strings.TrimSpace(ref.GroupID)
	a := strings.TrimSpace(ref.ArtifactID)
	v := strings.TrimSpace(ref.Version)
	if g == "" {
		g = child.Group
	}
	if a == "" || v == "" || strings.Contains(v, "${") {
		return Coord{}, unsupportedPOM(owner, "a <parent> with no resolvable groupId/artifactId/version",
			g+"/"+a+" "+v)
	}
	return Coord{Group: g, Artifact: a, Version: v}, nil
}

// interpolate substitutes ${name} from props. It returns the FIRST name it
// could not resolve, so the caller can name-error on exactly that property.
func interpolate(s string, props map[string]string) (out, missing string) {
	const maxRounds = 16
	for round := 0; round < maxRounds; round++ {
		i := strings.Index(s, "${")
		if i < 0 {
			return s, ""
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			return s, s[i:]
		}
		name := s[i+2 : i+j]
		val, ok := props[name]
		if !ok {
			return s, name
		}
		s = s[:i] + val + s[i+j+1:]
	}
	return s, "a ${…} chain nested deeper than " + itoa(maxRounds) + " levels"
}

// ---- edges ----------------------------------------------------------------

// pomChildren validates an effective POM and returns the dependency edges to
// follow. Validation happens in two passes so that a refused feature is only
// reported when it actually affects a dependency we WOULD follow: an
// unresolvable `${property}` version on a test-scoped or excluded edge is not
// our problem.
func pomChildren(e *effPOM, c Coord, inherited []string) ([]pomEdge, []Coord, error) {
	// Whole-POM features first — these change the meaning of every edge.
	if len(e.profiles) > 0 {
		return nil, nil, unsupportedPOM(c, "a <profile> that can change the dependency graph", e.profiles[0].ID)
	}
	if pk := e.packaging; pk != "" && pk != "jar" && pk != "bundle" {
		return nil, nil, unsupportedPOM(c, "<packaging> other than jar/bundle", pk)
	}

	var edges []pomEdge
	var pruned []Coord
	for _, d := range e.deps {
		g := strings.TrimSpace(d.GroupID)
		a := strings.TrimSpace(d.ArtifactID)
		v := strings.TrimSpace(d.Version)
		if g == "" || a == "" {
			continue
		}
		key := g + "/" + a

		if skippedScopes[strings.TrimSpace(d.Scope)] {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(d.Optional), "true") {
			continue
		}
		if excluded(key, inherited) {
			continue
		}
		// The clojure-itself prune runs BEFORE version resolution, deliberately:
		// it is what removes the single most common ${clojure.version} case in
		// the wild (spike s50 hit exactly that 404 on core.match).
		if clojureItself[key] {
			pruned = append(pruned, Coord{Group: g, Artifact: a, Version: v})
			continue
		}

		// <dependencyManagement> supplies a missing version...
		if v == "" {
			v = e.managed[key]
		}
		// ...and ${property} interpolates from the merged property map.
		if strings.Contains(v, "${") {
			resolved, missing := interpolate(v, e.props)
			if missing != "" {
				return nil, nil, unsupportedPOM(c, "${property} interpolation in a <version>",
					key+" "+v+" — no <properties> entry defines "+missing)
			}
			v = resolved
		}

		if err := validateEdgeVersion(c, key, v, e.managed[key] != ""); err != nil {
			return nil, nil, err
		}
		if cl := strings.TrimSpace(d.Classifier); cl != "" {
			return nil, nil, unsupportedPOM(c, "<classifier>", key+" classifier "+cl)
		}
		if t := strings.TrimSpace(d.Type); t != "" && t != "jar" && t != "bundle" {
			return nil, nil, unsupportedPOM(c, "<type> other than jar/bundle", key+" type "+t)
		}

		ed := pomEdge{Coord: Coord{Group: g, Artifact: a, Version: v}}
		ed.Exclusions = append(ed.Exclusions, inherited...)
		for _, x := range d.Exclusions {
			xg := strings.TrimSpace(x.GroupID)
			xa := strings.TrimSpace(x.ArtifactID)
			if xg == "" {
				xg = "*"
			}
			if xa == "" {
				xa = "*"
			}
			ed.Exclusions = append(ed.Exclusions, xg+":"+xa)
		}
		edges = append(edges, ed)
	}
	return edges, pruned, nil
}

// unsupportedVersionSyntax names the version shapes cljgo will not guess at,
// or "" when v is an ordinary fixed version. It is the ONE place that
// knowledge lives, so a user-declared version and a transitive POM edge are
// judged by exactly the same rule (adversarial verification found the
// user-declared side unvalidated, which blamed the repository for the user's
// syntax).
func unsupportedVersionSyntax(v string) string {
	switch {
	case strings.Contains(v, "${"):
		return "a ${property} placeholder"
	case v == "":
		return "an empty version"
	case strings.HasPrefix(v, "[") || strings.HasPrefix(v, "("):
		return "a version range"
	case strings.HasSuffix(v, "-SNAPSHOT"):
		return "a -SNAPSHOT version"
	case v == "LATEST" || v == "RELEASE":
		return "the floating meta-version " + v
	case strings.ContainsAny(v, `/\\`) || strings.Contains(v, ".."):
		// A version is a path SEGMENT in the repository URL. Anything with a
		// separator or a parent-dir hop is not a version; refusing it here
		// keeps it out of the fetch URL entirely rather than relying on the
		// remote to 404. (The on-disk cache path is a sha256, so this was not
		// a local traversal — but the rule belongs at the declaration.)
		return "a path separator in a version"
	}
	return ""
}

// validateEdgeVersion refuses those shapes on a transitive POM edge.
func validateEdgeVersion(owner Coord, key, v string, isManaged bool) error {
	if v == "" {
		if isManaged {
			return unsupportedPOM(owner, "<dependencyManagement> supplying a missing <version>", key)
		}
		return unsupportedPOM(owner, "a <dependency> with no <version>", key)
	}
	if bad := unsupportedVersionSyntax(v); bad != "" {
		return unsupportedPOM(owner, bad+" in a <version>", key+" "+v)
	}
	return nil
}

// validateDeclaredVersion refuses the same shapes when the USER writes them in
// build.cljgo. Without this, {:mvn/version "1.0-SNAPSHOT"} produced G5010
// "not found in any repository", blaming the repository for a syntax cljgo
// never supported.
func validateDeclaredVersion(name, v string) error {
	bad := unsupportedVersionSyntax(v)
	if bad == "" {
		return nil
	}
	return codedf("G5018", "dependency %q declares %s, which cljgo does not support", name, bad).
		withExpectedFound("a fixed published version, e.g. \"1.1.230\"", v).
		withFix("pin one published release, e.g. (dep b \"" + name + "\" {:mvn/version \"1.2.3\"})").
		withFix("or depend on it via :git — a ref is a fixed identity too")
}

// unsupportedPOM builds the G5011 every refusal shares.
func unsupportedPOM(owner Coord, feature, detail string) *DiagError {
	msg := "unsupported Maven POM feature in " + owner.String() + ": " + feature
	if detail != "" {
		msg += " (" + detail + ")"
	}
	// NOTE: the Fix deliberately does NOT suggest accept-version. That is a
	// version-CONFLICT override; it cannot supply a parent POM, a property or
	// a managed version, and a wrong Fix is worse than no Fix (CLAUDE.md).
	return codedf("G5011", "%s", msg).
		withFix("depend on " + owner.Key() + " via :git or :path instead, so cljgo resolves no POM for it").
		withFix("or vendor its source under vendor/" + owner.Key())
}

// excluded reports whether "group/artifact" is matched by any inherited
// exclusion pattern, `*` wildcards included.
func excluded(key string, patterns []string) bool {
	g, a, ok := splitCoordName(key)
	if !ok {
		return false
	}
	for _, p := range patterns {
		pg, pa, found := strings.Cut(p, ":")
		if !found {
			continue
		}
		if (pg == "*" || pg == g) && (pa == "*" || pa == a) {
			return true
		}
	}
	return false
}
