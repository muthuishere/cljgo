// conformance runs the golden syntax-quote corpus against the candidate
// reader. This is the skeleton of pkg/reader's CI gate: today the candidate
// is a stub that returns "NOT IMPLEMENTED", so every case fails — the point
// of the spike is that the ground truth and the diff loop already work.
//
// When pkg/reader lands, replace readStringStub with:
//
//	func readString(src string) (string, error) {
//	    form, err := reader.ReadString(src)
//	    if err != nil { return "", err }
//	    return lang.PrStr(form), nil
//	}
//
// Usage: conformance [-golden golden.txt] [-v]
// Exit status: number of failing cases capped at 1 (0 = fully conformant).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/muthuishere/cljgo/spikes/s8-syntax-quote-harness/harness"
)

// readStringStub is the injection point placeholder for pkg/reader.
func readStringStub(src string) (string, error) {
	return "NOT IMPLEMENTED", nil
}

func main() {
	goldenPath := flag.String("golden", "golden.txt", "path to normalized golden file")
	verbose := flag.Bool("v", false, "print passing cases too")
	flag.Parse()

	cases, err := harness.LoadGolden(*goldenPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load golden:", err)
		os.Exit(2)
	}
	fmt.Printf("syntax-quote conformance: %d cases from %s\n\n", len(cases), *goldenPath)

	rep := harness.Run(cases, readStringStub)
	if rep.Print(os.Stdout, *verbose) > 0 {
		os.Exit(1)
	}
}
