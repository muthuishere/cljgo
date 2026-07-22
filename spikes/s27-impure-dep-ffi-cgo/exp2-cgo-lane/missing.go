//go:build missinglib

// Same shape, but linking a C library that does NOT exist on this host —
// the "consumer's machine lacks the dep's system library" case.
package main

/*
#cgo LDFLAGS: -lnosuchlib_s27
extern int s27_missing_symbol(void);
*/
import "C"
import "fmt"

func version() string { return fmt.Sprint(int(C.s27_missing_symbol())) }
func main()           { fmt.Println("version:", version()) }
