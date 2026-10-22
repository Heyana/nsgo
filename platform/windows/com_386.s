//go:build windows

#include "textflag.h"

// doublecall is called by syscall.SyscallN, in the C convention, with a
// pointer to a doubleFrame: the method, its object, then room for a
// double. It calls the method with the object, passed in the slot below
// the saved BX and BP, and stores the double it returns in ST(0). BP
// keeps the stack pointer, whatever the method pops.
TEXT doublecall<>(SB),NOSPLIT|NOFRAME,$0
	MOVL	4(SP), AX
	SUBL	$12, SP
	MOVL	BX, 8(SP)
	MOVL	BP, 4(SP)
	MOVL	AX, BX
	MOVL	SP, BP
	MOVL	4(BX), CX
	MOVL	CX, 0(SP)
	MOVL	0(BX), AX
	CALL	AX
	FMOVDP	F0, 8(BX)
	MOVL	BP, SP
	MOVL	4(SP), BP
	MOVL	8(SP), BX
	ADDL	$12, SP
	RET

GLOBL	·doubleCallAddr(SB), RODATA, $4
DATA	·doubleCallAddr+0(SB)/4, $doublecall<>(SB)
