// Pure-Go twin of cgobaseline, for build-time comparison.
package main

import "fmt"

func add42(x int) int { return x + 42 }

func main() {
	fmt.Println("pure add42(100) =", add42(100))
}
