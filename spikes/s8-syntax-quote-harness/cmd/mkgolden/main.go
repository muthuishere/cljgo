// mkgolden turns the RAW golden file emitted by gen_golden.clj (per-JVM-run
// gensym numbers) into the committed, normalized golden file. Normalization
// uses the same normalize.Gensyms the diff harness applies to candidate
// output, so goldens are stable across regeneration runs: regenerating with a
// fresh JVM produces a byte-identical golden.txt.
//
// Usage: mkgolden golden.raw.txt golden.txt
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/spikes/s8-syntax-quote-harness/normalize"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mkgolden <raw-golden-in> <golden-out>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var out strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "OK: ") {
			// Normalize per case: numbering restarts at 1 for every record.
			line = "OK: " + normalize.Gensyms(line[len("OK: "):])
		} else if strings.HasPrefix(line, ";;") && strings.Contains(line, "RAW") {
			line = strings.Replace(line, "RAW (unnormalized gensym ids)", "NORMALIZED by cmd/mkgolden (gensym ids renumbered per case)", 1)
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	// strings.Split leaves one trailing empty element for the final \n; trim
	// the extra newline we just added for it.
	res := strings.TrimSuffix(out.String(), "\n")
	if err := os.WriteFile(os.Args[2], []byte(res), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
