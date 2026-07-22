package main

import (
	"bufio"
	"os"
	"strings"
)

// readProperties parses a Java-style .properties file into the SAME canonical
// Map an .edn file would (criterion 2): dotted keys nest, values coerce to
// typed scalars.
//
//	db.pool-size=10
//	db.host=localhost
//	features.signup=true
//
// becomes {"db":{"pool-size":10,"host":"localhost"},"features":{"signup":true}}
//
// This is the Java-familiarity affordance: a team that reaches for
// application.properties gets the identical resolved tree as application.edn.
// Missing file => (nil,nil), same no-contribution semantics as readEDN.
func readProperties(path string) (Map, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := Map{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		// key=value ; ':' is also a legal Java separator.
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			continue
		}
		key := strings.TrimSpace(line[:sep])
		val := strings.TrimSpace(line[sep+1:])
		out = setIn(out, dottedPath(key), coerceScalar(val))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// dottedPath maps "db.pool-size" -> ["db","pool-size"]. Segments keep their
// kebab case verbatim, matching EDN keyword names.
func dottedPath(key string) Path {
	parts := strings.Split(key, ".")
	p := make(Path, 0, len(parts))
	for _, s := range parts {
		if s = strings.TrimSpace(s); s != "" {
			p = append(p, s)
		}
	}
	return p
}
