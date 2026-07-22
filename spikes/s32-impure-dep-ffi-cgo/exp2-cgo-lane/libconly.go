//go:build libconly

// cgo against libc ONLY — no third-party system library. This is the case
// zig-cc genuinely rescues, and the control for the sqlite3 failure.
package main

/*
#include <stdlib.h>
*/
import "C"
import "fmt"

func version() string { return fmt.Sprint(int(C.abs(C.int(-42)))) }
func main()           { fmt.Println("version:", version()) }
