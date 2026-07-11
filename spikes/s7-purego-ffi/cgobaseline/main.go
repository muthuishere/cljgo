// S7 priority-5 sanity check: CGO_ENABLED=1 must work on this machine.
package main

/*
int add42(int x) { return x + 42; }
*/
import "C"
import "fmt"

func main() {
	fmt.Println("cgo add42(100) =", int(C.add42(100)))
}
