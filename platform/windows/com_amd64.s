//go:build windows

#include "textflag.h"

// doublecall is called by syscall.SyscallN, in the C convention on the
// system stack, with CX pointing to a doubleFrame: the method, its object,
// then room for a double. It calls the method with the object, leaving
// the shadow space the callee may use, and stores the double it returns
// in X0. BX, which keeps the frame, is the caller's and is saved.
TEXT doublecall<>(SB),NOSPLIT|NOFRAME,$0
	PUSHQ	BX
	MOVQ	CX, BX
	SUBQ	$32, SP
	MOVQ	8(BX), CX
	MOVQ	0(BX), AX
	CALL	AX
	MOVSD	X0, 16(BX)
	ADDQ	$32, SP
	POPQ	BX
	RET

GLOBL	·doubleCallAddr(SB), RODATA, $8
DATA	·doubleCallAddr+0(SB)/8, $doublecall<>(SB)
