#include "textflag.h"

// Each array contains 64 int16 values = 128 bytes.
// One YMM register holds 16 int16 values = 32 bytes.
// Therefore the loop executes four times.
//
// dst += to - from - captured
//
// func captureAVX2(
//     a0, a1,
//     wTo0, wFrom0, wCap0,
//     wTo1, wFrom1, wCap1 *[64]int16,
// )
TEXT ·captureAVX2(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0 - wCap0
	VMOVDQU (AX)(R10*1), Y0
	VPADDW  (CX)(R10*1), Y0, Y0
	VPSUBW  (DX)(R10*1), Y0, Y0
	VPSUBW  (SI)(R10*1), Y0, Y0
	VMOVDQU Y0, (AX)(R10*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1 - wCap1
	VMOVDQU (BX)(R10*1), Y1
	VPADDW  (DI)(R10*1), Y1, Y1
	VPSUBW  (R8)(R10*1), Y1, Y1
	VPSUBW  (R9)(R10*1), Y1, Y1
	VMOVDQU Y1, (BX)(R10*1)

	ADDQ $32, R10
	CMPQ R10, $128
	JB loop

	VZEROUPPER
	RET

	// func moveAVX2(
//     a0, a1,
//     wFrom0, wTo0,
//     wFrom1, wTo1 *[64]int16,
// )
TEXT ·moveAVX2(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop:
	// Perspective 0:
	// a0 += wTo0 - wFrom0
	VMOVDQU (AX)(R8*1), Y0
	VPADDW  (DX)(R8*1), Y0, Y0
	VPSUBW  (CX)(R8*1), Y0, Y0
	VMOVDQU Y0, (AX)(R8*1)

	// Perspective 1:
	// a1 += wTo1 - wFrom1
	VMOVDQU (BX)(R8*1), Y1
	VPADDW  (DI)(R8*1), Y1, Y1
	VPSUBW  (SI)(R8*1), Y1, Y1
	VMOVDQU Y1, (BX)(R8*1)

	ADDQ $32, R8
	CMPQ R8, $128
	JB move_loop

	VZEROUPPER
	RET

	// func castleAVX2(
//     a0, a1,
//     wKFrom0, wKTo0, wRFrom0, wRTo0,
//     wKFrom1, wKTo1, wRFrom1, wRTo1 *[64]int16,
// )
TEXT ·castleAVX2(SB), NOSPLIT, $0-80
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wKFrom0+16(FP), CX
	MOVQ wKTo0+24(FP), DX
	MOVQ wRFrom0+32(FP), SI
	MOVQ wRTo0+40(FP), DI

	MOVQ wKFrom1+48(FP), R8
	MOVQ wKTo1+56(FP), R9
	MOVQ wRFrom1+64(FP), R10
	MOVQ wRTo1+72(FP), R11

	XORQ R12, R12

castle_loop:
	// Perspective 0:
	// a0 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (AX)(R12*1), Y0
	VPADDW  (DX)(R12*1), Y0, Y0
	VPSUBW  (CX)(R12*1), Y0, Y0
	VPADDW  (DI)(R12*1), Y0, Y0
	VPSUBW  (SI)(R12*1), Y0, Y0
	VMOVDQU Y0, (AX)(R12*1)

	// Perspective 1:
	// a1 += kingTo - kingFrom + rookTo - rookFrom
	VMOVDQU (BX)(R12*1), Y1
	VPADDW  (R9)(R12*1), Y1, Y1
	VPSUBW  (R8)(R12*1), Y1, Y1
	VPADDW  (R11)(R12*1), Y1, Y1
	VPSUBW  (R10)(R12*1), Y1, Y1
	VMOVDQU Y1, (BX)(R12*1)

	ADDQ $32, R12
	CMPQ R12, $128
	JB castle_loop

	VZEROUPPER
	RET
