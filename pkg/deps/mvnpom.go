package deps

// The .pom reader: a deliberately SMALL, pure-Go subset of Maven (encoding/xml
// only — no JVM, no mvn, no Aether), plus a validator that NAME-ERRORS every
// feature outside the subset instead of half-supporting it.
//
// That refusal is spike s50 finding 1, binding: `${property}` interpolation and
// dependencyManagement version supply must name-error, because an
// uninterpolated or absent version is a WRONG version and a clear diagnostic
// beats a silent wrong answer. Each G5011 is raised at the coordinate that
// needs the feature, so the message can say which library, not "a pom failed".

import (
	"encoding/xml"
	"strings"
)

// pomXML is the parsed subset of a POM.
type pomXML struct {
	XMLName    xml.Name `xml:"project"`
	Parent     *pomRef  `xml:"parent"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Version    string   `xml:"version"`
	Packaging  string   `xml:"packaging"`

	// Presence-only: any of these means a feature we refuse to guess at.
	DependencyManagement *struct {
		Dependencies []pomDepXML `xml:"dependencies>dependency"`
	} `xml:"dependencyManagement"`
	Profiles []struct {
		ID string `xml:"id"`
	} `xml:"profiles>profile"`

	Dependencies []pomDepXML `xml:"dependencies>dependency"`
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
	if err := xml.Unmarshal(b, &p); err != nil {
		return nil, codedf("G5011", "cannot parse the POM of %s: %v", c, err).
			withFix("the repository served something that is not a Maven POM; check the coordinate, or depend via :git")
	}
	return &p, nil
}

// pomChildren validates a POM and returns the dependency edges to follow.
// Validation happens in two passes so that a refused feature is only reported
// when it actually affects a dependency we WOULD follow: a `${property}`
// version on a test-scoped or excluded edge is not our problem.
func pomChildren(p *pomXML, c Coord, inherited []string) ([]pomEdge, []Coord, error) {
	// Whole-POM features first — these change the meaning of every edge.
	if p.Parent != nil && (p.Parent.GroupID != "" || p.Parent.ArtifactID != "") {
		return nil, nil, unsupportedPOM(c, "<parent> POM inheritance",
			p.Parent.GroupID+"/"+p.Parent.ArtifactID)
	}
	if len(p.Profiles) > 0 {
		return nil, nil, unsupportedPOM(c, "<profiles>", p.Profiles[0].ID)
	}
	if pk := strings.TrimSpace(p.Packaging); pk != "" && pk != "jar" && pk != "bundle" {
		return nil, nil, unsupportedPOM(c, "<packaging> other than jar/bundle", pk)
	}

	managed := map[string]bool{}
	if p.DependencyManagement != nil {
		for _, d := range p.DependencyManagement.Dependencies {
			managed[strings.TrimSpace(d.GroupID)+"/"+strings.TrimSpace(d.ArtifactID)] = true
		}
	}

	var edges []pomEdge
	var pruned []Coord
	for _, d := range p.Dependencies {
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
		// The clojure-itself prune runs BEFORE validation, deliberately: it is
		// what removes the single most common ${clojure.version} case in the
		// wild (spike s50 hit exactly that 404 on core.match).
		if clojureItself[key] {
			pruned = append(pruned, Coord{Group: g, Artifact: a, Version: v})
			continue
		}

		if err := validateEdgeVersion(c, key, v, managed[key]); err != nil {
			return nil, nil, err
		}
		if cl := strings.TrimSpace(d.Classifier); cl != "" {
			return nil, nil, unsupportedPOM(c, "<classifier>", key+" classifier "+cl)
		}
		if t := strings.TrimSpace(d.Type); t != "" && t != "jar" && t != "bundle" {
			return nil, nil, unsupportedPOM(c, "<type> other than jar/bundle", key+" type "+t)
		}

		e := pomEdge{Coord: Coord{Group: g, Artifact: a, Version: v}}
		e.Exclusions = append(e.Exclusions, inherited...)
		for _, x := range d.Exclusions {
			xg := strings.TrimSpace(x.GroupID)
			xa := strings.TrimSpace(x.ArtifactID)
			if xg == "" {
				xg = "*"
			}
			if xa == "" {
				xa = "*"
			}
			e.Exclusions = append(e.Exclusions, xg+":"+xa)
		}
		edges = append(edges, e)
	}
	return edges, pruned, nil
}

// validateEdgeVersion refuses the version shapes cljgo will not guess at.
func validateEdgeVersion(owner Coord, key, v string, isManaged bool) error {
	switch {
	case strings.Contains(v, "${"):
		return unsupportedPOM(owner, "${property} interpolation in a <version>", key+" "+v)
	case v == "" && isManaged:
		return unsupportedPOM(owner, "<dependencyManagement> supplying a missing <version>", key)
	case v == "":
		return unsupportedPOM(owner, "a <dependency> with no <version>", key)
	case strings.HasPrefix(v, "[") || strings.HasPrefix(v, "("):
		return unsupportedPOM(owner, "a version range", key+" "+v)
	case strings.HasSuffix(v, "-SNAPSHOT"):
		return unsupportedPOM(owner, "a -SNAPSHOT version", key+" "+v)
	}
	return nil
}

// unsupportedPOM builds the G5011 every refusal shares.
func unsupportedPOM(owner Coord, feature, detail string) *DiagError {
	msg := "unsupported Maven POM feature in " + owner.String() + ": " + feature
	if detail != "" {
		msg += " (" + detail + ")"
	}
	return codedf("G5011", "%s", msg).
		withFix("pin the version yourself with (accept-version b \"" + owner.Key() + "\" \"" + owner.Version + "\"), or depend on this library via :git or :path instead")
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
