//go:build go1.26 && !go1.27

#include "textflag.h"

// func getg() unsafe.Pointer
//
// On amd64 the current goroutine's g pointer lives in thread-local
// storage; Go assembly exposes it as the (TLS) pseudo-address
// (go.dev/doc/asm: "MOVQ (TLS), AX ... loads g into AX").
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVQ (TLS), AX
	MOVQ AX, ret+0(FP)
	RET
