//go:build windows

#include "textflag.h"

// The C convention passes ElementProviderFromPoint's x and y in D0 and D1
// and its out pointer in X1, and RangeValue's SetValue's value in D0; the
// callbacks read X0 to X7 only. These move the doubles' bits after the
// object and go on to the callback.
TEXT uiafrompoint<>(SB),NOSPLIT|NOFRAME,$0
	MOVD	R1, R3
	FMOVD	F0, R1
	FMOVD	F1, R2
	MOVD	·uiaFromPointTarget(SB), R16
	JMP	(R16)

TEXT uiasetrange<>(SB),NOSPLIT|NOFRAME,$0
	FMOVD	F0, R1
	MOVD	·uiaSetRangeTarget(SB), R16
	JMP	(R16)

GLOBL	·uiaFromPointThunkAddr(SB), NOPTR, $8
DATA	·uiaFromPointThunkAddr+0(SB)/8, $uiafrompoint<>(SB)
GLOBL	·uiaSetRangeThunkAddr(SB), NOPTR, $8
DATA	·uiaSetRangeThunkAddr+0(SB)/8, $uiasetrange<>(SB)
