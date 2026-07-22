module s30proto

go 1.26

// Read-only use of the repo's reader/lang packages to prototype a syntactic
// scan. The spike NEVER modifies pkg/ (ADR 0027); this replace only READS it.
require github.com/muthuishere/cljgo v0.0.0

replace github.com/muthuishere/cljgo => ../../..
