//go:build windows

#include "textflag.h"

// The C convention passes ElementProviderFromPoint's x and y in X1 and X2
// and RangeValue's SetValue's value in X1, where the callbacks, which
// read RCX, RDX, R8 and R9, do not look: these move them to RDX and R8
// and go on to the callback.
TEXT uiafrompoint<>(SB),NOSPLIT|NOFRAME,$0
	MOVQ	X1, DX
	MOVQ	X2, R8
	MOVQ	·uiaFromPointTarget(SB), AX
	JMP	AX

TEXT uiasetrange<>(SB),NOSPLIT|NOFRAME,$0
	MOVQ	X1, DX
	MOVQ	·uiaSetRangeTarget(SB), AX
	JMP	AX

GLOBL	·uiaFromPointThunkAddr(SB), NOPTR, $8
DATA	·uiaFromPointThunkAddr+0(SB)/8, $uiafrompoint<>(SB)
GLOBL	·uiaSetRangeThunkAddr(SB), NOPTR, $8
DATA	·uiaSetRangeThunkAddr+0(SB)/8, $uiasetrange<>(SB)
