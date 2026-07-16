package format14

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OracleResult is what real JVM Clojure 1.12.5 did with one probe.
type OracleResult struct {
	Output  string // exact stdout, present iff !Threw
	Threw   bool
	ExClass string // simple class name, present iff Threw
}

const probeMarker = "===PROBE:"
const okMarker = "<<<OK>>>"
const errMarkerPrefix = "<<<ERR:"
const errMarkerSuffix = ">>>"

// buildOracleScript emits ONE .clj file covering the whole corpus so we pay
// JVM boot cost once (~1-2s), not once per probe (~90 probes x 1.5s would be
// over two minutes of pure boot tax).
func buildOracleScript(probes []Probe) string {
	var b strings.Builder
	b.WriteString("(ns s14-oracle)\n")
	for _, p := range probes {
		b.WriteString(fmt.Sprintf("(println %q)\n", probeMarker+p.Name+"===")) //nolint
		b.WriteString("(try\n")
		if p.ArgsClj == "" {
			b.WriteString(fmt.Sprintf("  (print (format %q))\n", p.Fmt))
		} else {
			b.WriteString(fmt.Sprintf("  (print (format %q %s))\n", p.Fmt, p.ArgsClj))
		}
		b.WriteString(fmt.Sprintf("  (print %q)\n", "\n"+okMarker))
		b.WriteString("  (catch Throwable e\n")
		b.WriteString(fmt.Sprintf("    (print (str %q (.getSimpleName (class e)) %q))))\n", errMarkerPrefix, errMarkerSuffix))
		b.WriteString("(println)\n")
	}
	return b.String()
}

// RunOracle shells out to the real `clojure` CLI exactly once for the whole
// corpus and parses per-probe stdout/exception results.
func RunOracle(probes []Probe) (map[string]OracleResult, error) {
	script := buildOracleScript(probes)
	dir, err := os.MkdirTemp("", "s14-oracle")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	scriptPath := filepath.Join(dir, "oracle.clj")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("clojure", "-M", "-e", fmt.Sprintf("(load-file %q)", scriptPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		// clojure prints WARNING lines to stderr sometimes even on success;
		// only fail hard if we got no markers at all.
		if !strings.Contains(string(out), probeMarker) {
			return nil, fmt.Errorf("clojure invocation failed: %w\n%s", err, out)
		}
	}

	return parseOracleOutput(string(out), probes), nil
}

func parseOracleOutput(out string, probes []Probe) map[string]OracleResult {
	results := make(map[string]OracleResult, len(probes))
	// Split on probe markers; first chunk (before first marker) is boot noise.
	sections := strings.Split(out, probeMarker)
	for _, sec := range sections[1:] {
		nameEnd := strings.Index(sec, "===\n")
		if nameEnd < 0 {
			continue
		}
		name := sec[:nameEnd]
		body := sec[nameEnd+4:]

		var res OracleResult
		if idx := strings.Index(body, errMarkerPrefix); idx >= 0 {
			rest := body[idx+len(errMarkerPrefix):]
			end := strings.Index(rest, errMarkerSuffix)
			if end >= 0 {
				res.Threw = true
				res.ExClass = rest[:end]
			}
		} else if idx := strings.Index(body, "\n"+okMarker); idx >= 0 {
			res.Output = body[:idx]
		} else if idx := strings.Index(body, okMarker); idx >= 0 {
			res.Output = body[:idx]
		}
		results[name] = res
	}
	return results
}
