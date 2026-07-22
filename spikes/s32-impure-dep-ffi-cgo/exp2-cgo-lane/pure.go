//go:build pure

// The PURE-Go control: same observable output, no C at all.
package main

import "fmt"

func version() string { return "pure-go-no-sqlite" }
func main()           { fmt.Println("version:", version()) }
