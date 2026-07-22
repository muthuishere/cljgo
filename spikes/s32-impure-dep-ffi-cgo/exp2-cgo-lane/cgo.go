//go:build cgolink

// The cgo lane: exactly what ADR 0021's `(c-link art {:pkg-config "sqlite3"})`
// would emit — a C dependency on a SYSTEM library, resolved by the host
// linker at build time.
package main

/*
#cgo LDFLAGS: -lsqlite3
#include <sqlite3.h>
*/
import "C"
import "fmt"

func version() string { return C.GoString(C.sqlite3_libversion()) }
func main()           { fmt.Println("version:", version()) }
