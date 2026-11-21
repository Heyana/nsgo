//go:build windows

#include "textflag.h"

// The Windows x64 ABI passes IDWriteTextRenderer::DrawGlyphRun's float32 baselineOriginX and baselineOriginY in XMM2 and XMM3 (parameters 2 and 3), while Go's syscall.NewCallback reads R8 and R9; move them and jump on.
TEXT textdrawglyphrun<>(SB),NOSPLIT|NOFRAME,$0
	MOVQ	X2, R8
	MOVQ	X3, R9
	MOVQ	·textDrawGlyphRunTarget(SB), AX
	JMP	AX

GLOBL	·textDrawGlyphRunThunkAddr(SB), NOPTR, $8
DATA	·textDrawGlyphRunThunkAddr+0(SB)/8, $textdrawglyphrun<>(SB)
