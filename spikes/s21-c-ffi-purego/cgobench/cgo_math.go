package cgobench

// #include <math.h>
import "C"

func cCos(x float64) float64 {
	return float64(C.cos(C.double(x)))
}
