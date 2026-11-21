//go:build windows

#include "textflag.h"

// The Windows ARM64 ABI passes IDWriteTextRenderer::DrawGlyphRun's float32 baselineOriginX and baselineOriginY in S0 and S1 (low halves of F0 and F1) and shifts parameters 4..7 into R2..R5; Go's syscall.NewCallback reads R0..R7 only. Shift R2..R5 to R4..R7, move F0/F1 into R2/R3, and jump on.
TEXT textdrawglyphrun<>(SB),NOSPLIT|NOFRAME,$0
	MOVD	R5, R7
	MOVD	R4, R6
	MOVD	R3, R5
	MOVD	R2, R4
	FMOVD	F0, R2
	FMOVD	F1, R3
	MOVD	·textDrawGlyphRunTarget(SB), R16
	JMP	(R16)

GLOBL	·textDrawGlyphRunThunkAddr(SB), NOPTR, $8
DATA	·textDrawGlyphRunThunkAddr+0(SB)/8, $textdrawglyphrun<>(SB)
