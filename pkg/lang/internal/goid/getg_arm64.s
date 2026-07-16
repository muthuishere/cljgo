//go:build go1.26 && !go1.27

#include "textflag.h"

// func getg() unsafe.Pointer
//
// On arm64 the current goroutine's g pointer is pinned in the
// dedicated g register (R28), which Go assembly names g.
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVD g, R0
	MOVD R0, ret+0(FP)
	RET
