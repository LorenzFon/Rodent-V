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
TEXT ·captureAVX2_64(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

capture_64_loop:
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
	JB capture_64_loop

	VZEROUPPER
	RET

	// func moveAVX2(
//     a0, a1,
//     wFrom0, wTo0,
//     wFrom1, wTo1 *[64]int16,
// )
TEXT ·moveAVX2_64(SB), NOSPLIT, $0-48
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
TEXT ·castleAVX2_64(SB), NOSPLIT, $0-80
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

// for 128 HL network

TEXT ·captureAVX2_128(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

// loop label should be unique per file
capture_128_loop:
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

    // loop limit is 256 = 2 * number of hidden neurons
	ADDQ $32, R10
	CMPQ R10, $256	
	JB capture_128_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_128(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_128:
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
	CMPQ R8, $256
	JB move_loop_128

	VZEROUPPER
	RET

// for 256 HL network

TEXT ·captureAVX2_256(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

// loop label should be unique per file
capture_256_loop:
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

    // loop limit is 512 = 2 * number of hidden neurons
	ADDQ $32, R10
	CMPQ R10, $512	
	JB capture_256_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_256(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_256:
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
	CMPQ R8, $512
	JB move_loop_256

	VZEROUPPER
	RET

// for 512 HL

TEXT ·captureAVX2_512(SB), NOSPLIT, $0-64
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wTo0+16(FP), CX
	MOVQ wFrom0+24(FP), DX
	MOVQ wCap0+32(FP), SI

	MOVQ wTo1+40(FP), DI
	MOVQ wFrom1+48(FP), R8
	MOVQ wCap1+56(FP), R9

	XORQ R10, R10

// loop label should be unique per file
capture_512_loop:
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

    // loop limit is 1024 = 2 * number of hidden neurons
	ADDQ $32, R10
	CMPQ R10, $1024	
	JB capture_512_loop

	VZEROUPPER
	RET

TEXT ·moveAVX2_512(SB), NOSPLIT, $0-48
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX

	MOVQ wFrom0+16(FP), CX
	MOVQ wTo0+24(FP), DX

	MOVQ wFrom1+32(FP), SI
	MOVQ wTo1+40(FP), DI

	XORQ R8, R8

move_loop_512:
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
	CMPQ R8, $1024
	JB move_loop_512

	VZEROUPPER
	RET

// EVAL

// func getEvalAVX2_64(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
//
// 64 int16 neurons = 128 bytes.
// Each loop iteration processes 16 neurons = 32 bytes.
TEXT ·getEvalAVX2_64(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

geteval_64_loop:
	// Perspective 0.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// Clamp accumulator values to [0, 255].
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	// Perspective 1.
	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	ADDQ $32, R9
	CMPQ R9, $128
	JB geteval_64_loop

	// Reduce eight int32 lanes to one.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

	// func getEvalAVX2_128(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
//
// 128 int16 neurons = 256 bytes.
// Each loop iteration processes 16 neurons = 32 bytes.
TEXT ·getEvalAVX2_128(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

geteval_128_loop:
	// Perspective 0.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// Clamp accumulator values to [0, 255].
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	// Perspective 1.
	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5
	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5
	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	ADDQ $32, R9
	CMPQ R9, $256
	JB geteval_128_loop

	// Reduce eight int32 lanes to one.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

// func getEvalAVX2_256(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
//
// For every neuron:
//
//     v = clamp(acc, 0, 255)
//     sum += v * v * weight
//
// 256 int16 neurons = 512 bytes.
// Each loop iteration processes 16 neurons.
TEXT ·getEvalAVX2_256(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = sixteen int16 zeros.
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values equal to 255.
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Y8 accumulates eight int32 partial sums.
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

geteval_256_loop:
	// ------------------------------------------------------------
	// Perspective 0
	// ------------------------------------------------------------

	// Load 16 accumulator values and 16 signed weights.
	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// SCReLU clipping: max(x, 0), then min(x, 255).
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight int16 values -> eight int32 values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	// v * v * weight.
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1
	// ------------------------------------------------------------

	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// Lower eight values.
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2
	VPADDD Y2, Y8, Y8

	// Upper eight values.
	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4
	VPADDD Y4, Y8, Y8

	ADDQ $32, R9
	CMPQ R9, $512
	JB geteval_256_loop

	// Horizontally reduce eight int32 lanes to one int32.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	// [a,b,c,d] + [c,d,a,b]
	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	// [a,b,...] + [b,a,...]
	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	// Store the low int32 result.
	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET

// func getEvalAVX2_512(
//     a0, a1 *int16,
//     w0, w1 *int16,
//     sum *int32,
// )
TEXT ·getEvalAVX2_512(SB), NOSPLIT, $0-40
	MOVQ a0+0(FP), AX
	MOVQ a1+8(FP), BX
	MOVQ w0+16(FP), CX
	MOVQ w1+24(FP), DX
	MOVQ sum+32(FP), SI

	// Y14 = zero
	VPXOR Y14, Y14, Y14

	// Y15 = sixteen int16 values containing 255
	MOVL $255, R8
	VMOVD R8, X15
	VPBROADCASTW X15, Y15

	// Y8 = int32 accumulation
	VPXOR Y8, Y8, Y8

	XORQ R9, R9

eval512_loop:
	// ------------------------------------------------------------
	// Perspective 0, lower eight neurons
	// ------------------------------------------------------------

	VMOVDQU (AX)(R9*1), Y0
	VMOVDQU (CX)(R9*1), Y1

	// clamp accumulator to [0, 255]
	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	// lower 8 x int16 -> int32
	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	// x² * weight
	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2

	VPADDD Y2, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 0, upper eight neurons
	// ------------------------------------------------------------

	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4

	VPADDD Y4, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1, lower eight neurons
	// ------------------------------------------------------------

	VMOVDQU (BX)(R9*1), Y0
	VMOVDQU (DX)(R9*1), Y1

	VPMAXSW Y14, Y0, Y0
	VPMINSW Y15, Y0, Y0

	VPMOVSXWD X0, Y2
	VPMOVSXWD X1, Y3

	VPMULLD Y2, Y2, Y2
	VPMULLD Y3, Y2, Y2

	VPADDD Y2, Y8, Y8

	// ------------------------------------------------------------
	// Perspective 1, upper eight neurons
	// ------------------------------------------------------------

	VEXTRACTI128 $1, Y0, X4
	VEXTRACTI128 $1, Y1, X5

	VPMOVSXWD X4, Y4
	VPMOVSXWD X5, Y5

	VPMULLD Y4, Y4, Y4
	VPMULLD Y5, Y4, Y4

	VPADDD Y4, Y8, Y8

	// 16 int16 neurons = 32 bytes
	ADDQ $32, R9

	// 512 int16 neurons = 1024 bytes
	CMPQ R9, $1024
	JL eval512_loop

	// Horizontal sum of eight int32 lanes in Y8.
	VEXTRACTI128 $1, Y8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0x4E, X8, X1
	VPADDD X1, X8, X8

	VPSHUFD $0xB1, X8, X1
	VPADDD X1, X8, X8

	VMOVD X8, R8
	MOVL R8, (SI)

	VZEROUPPER
	RET
	