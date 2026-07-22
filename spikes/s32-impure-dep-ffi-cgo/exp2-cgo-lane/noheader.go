//go:build noheader
package main

/*
#cgo LDFLAGS: -lsqlite3
#include <sqlite3_that_does_not_exist.h>
*/
import "C"
import "fmt"

func main() { fmt.Println("unreachable") }
