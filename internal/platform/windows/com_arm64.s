//go:build windows

#include "textflag.h"

// floatcall is called by syscall.SyscallN, in the C convention on the
// system stack, with R0 pointing to a floatFrame: the method, then the
// words of X0-X7, then those of D0-D7. It loads them and jumps to the
// method, which returns to SyscallN.
TEXT floatcall<>(SB),NOSPLIT|NOFRAME,$0
	MOVD	R0, R9
	FMOVD	72(R9), F0
	FMOVD	80(R9), F1
	FMOVD	88(R9), F2
	FMOVD	96(R9), F3
	FMOVD	104(R9), F4
	FMOVD	112(R9), F5
	FMOVD	120(R9), F6
	FMOVD	128(R9), F7
	MOVD	8(R9), R0
	MOVD	16(R9), R1
	MOVD	24(R9), R2
	MOVD	32(R9), R3
	MOVD	40(R9), R4
	MOVD	48(R9), R5
	MOVD	56(R9), R6
	MOVD	64(R9), R7
	MOVD	0(R9), R9
	JMP	(R9)

GLOBL	·floatCallAddr(SB), RODATA, $8
DATA	·floatCallAddr+0(SB)/8, $floatcall<>(SB)

// doublecall is called like floatcall, with R0 pointing to a doubleFrame:
// the method, its object, then room for a double. It calls the method
// with the object and stores the double it returns in D0. It saves R19,
// which it keeps the frame in, and the link register.
TEXT doublecall<>(SB),NOSPLIT|NOFRAME,$0
	SUB	$16, RSP
	STP	(R19, R30), (RSP)
	MOVD	R0, R19
	MOVD	8(R19), R0
	MOVD	0(R19), R9
	CALL	(R9)
	FMOVD	F0, 16(R19)
	LDP	(RSP), (R19, R30)
	ADD	$16, RSP
	RET

GLOBL	·doubleCallAddr(SB), RODATA, $8
DATA	·doubleCallAddr+0(SB)/8, $doublecall<>(SB)
