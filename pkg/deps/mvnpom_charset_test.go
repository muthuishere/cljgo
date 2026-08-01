package deps

import (
	"strings"
	"testing"
)

// TestParsePOMAcceptsNonUTF8 — a POM declaring ISO-8859-1 must parse.
//
// encoding/xml refuses any declared encoding other than UTF-8 unless a
// CharsetReader is supplied, failing with `xml: encoding "ISO-8859-1"
// declared but Decoder.CharsetReader is nil`. That Go implementation detail
// leaked verbatim into a G5011 telling the user "the repository served
// something that is not a Maven POM" — when it IS one, and nothing about
// their coordinate was wrong. Real artifacts hit this (commons-parent;
// ~1.4% of the poms cached on one machine, spike s79) and the user has no
// way to act on it.
func TestParsePOMAcceptsNonUTF8(t *testing.T) {
	pom := `<?xml version="1.0" encoding="ISO-8859-1"?>
<project>
  <groupId>org.apache.commons</groupId>
  <artifactId>commons-parent</artifactId>
  <version>52</version>
  <description>Caf` + "\xe9" + ` — a Latin-1 byte in prose</description>
</project>`
	got, err := parsePOM([]byte(pom), Coord{Group: "org.apache.commons", Artifact: "commons-parent", Version: "52"})
	if err != nil {
		t.Fatalf("ISO-8859-1 POM failed to parse: %v", err)
	}
	if got.ArtifactID != "commons-parent" {
		t.Errorf("artifactId = %q, want commons-parent", got.ArtifactID)
	}
	if got.Version != "52" {
		t.Errorf("version = %q, want 52", got.Version)
	}
}

// TestParsePOMStillRejectsNonPOM keeps the real diagnostic honest: genuinely
// malformed input must still produce the coded error, not be waved through.
func TestParsePOMStillRejectsNonPOM(t *testing.T) {
	_, err := parsePOM([]byte("<html><body>404 not found"), Coord{Group: "a", Artifact: "b", Version: "1"})
	if err == nil {
		t.Fatal("malformed POM parsed without error")
	}
	if !strings.Contains(err.Error(), "G5011") {
		t.Errorf("error does not carry G5011: %v", err)
	}
}
