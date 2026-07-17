package cgobench

import "testing"

// The cgo baseline for the same libm cos(x) call the parent module's
// BenchmarkPuregoStatic/BenchmarkPuregoDynamic measure — separate module
// because it needs CGO_ENABLED=1 (this spike's headline module deliberately
// builds with CGO_ENABLED=0 to prove purego needs no C toolchain).
//
// Run: CGO_ENABLED=1 go test -bench=. -benchtime=2s -run=^$ .

var sink float64

func BenchmarkCgoCos(b *testing.B) {
	x := 0.5
	for i := 0; i < b.N; i++ {
		x = cCos(x)
	}
	sink = x
}
