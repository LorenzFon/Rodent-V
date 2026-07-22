package main

/*

Rodent supports networks trained by bullet simple.rs.
The net architecture is 768 -> (N)x2 -> 1, allowing
some variance of N.

Code is optimized and tries to run SIMD version if possible;
if not, it defaults to much slower scalar code. State of the
network is kept in the "accumulator", kept on accStack
within the search state. Updates are following copy-make
principle: old accumulator is copied to the child node
and modified there; trying a new move requires copying
the accumulator yet again. Within the parent node, accumulator
update is delayed as much as possible, and not performed
on illegal or pruned moves.

 build command for AVX2 version
 set GOAMD64=v3
 go build

// If you want to build a portable version, avoid setting GOAMD64=v3
   If you miss cpu detection, run: go get golang.org/x/sys/cpu
*/

import (
	_ "embed"
	"os"
	"unsafe"

	"golang.org/x/sys/cpu"
)

//go:embed nets/rodent_v_128hl_2.bin
var embeddedNet []byte

// NNUE size and scale. AVX2 code supports following net sizes:
// 64, 128, 256, 512
const (
	NNUEInputSize  = 768
	NNUEHiddenSize = 128
	NNUEEvalScale  = 400
	NNUEL0Scale    = 255
	NNUEL1Scale    = 128
)

// Types of NNUE updates
type AccUpdateKind int

const (
	uNORMAL AccUpdateKind = iota
	uCASTLE
	uEP_CAP
	uEP_SET
	uCAPTURE
	uPROMO
	uPROMCAPT
)

// Params for NNUE evaluation
type NNUEParameters struct {
	InputWeights  [NNUEInputSize][NNUEHiddenSize]int16
	InputBiases   [NNUEHiddenSize]int16
	OutputWeights [2][NNUEHiddenSize]int16
	OutputBias    int16
}

var nnueParams = &NNUEParameters{}
var nnue NNUEState

type Accumulator struct {
	values [2][NNUEHiddenSize]int16
}

// Update contains data for, well, updating nnue accumulator.
// Data are stored on a stack, created in makeMove() and used
// to postpone accumulator update *within a single node*.
// In practice it means that we can avoid the update when
// a move is illegal (leaves us in check) or if it is pruned.
type Update struct {

	// Data used by NNUE accumulator.
	dirty      bool
	color      int
	flag       AccUpdateKind
	from       int
	to         int
	capSq      int
	movingType int
	captType   int
	prom       int
	rookFrom   int // for castling
	rookTo     int
}

// The accumulator itself now belongs to searchState.
// This state contains only information shared by the engine.
type NNUEState struct {
	Loaded bool
}

// Interface to cater for assembly code for various net sizes.

type captureFunc func(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

type moveFunc func(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

type castleFunc func(
	a0, a1 *int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *int16,
)

type evalFunc func(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

var captureFunction captureFunc
var moveFunction moveFunc
var castleFunction castleFunc
var evalFunction evalFunc

// init picks the correct assembly routines for the configured NNUE size.
func init() {
	// Safe fallback.
	moveFunction = moveScalar
	captureFunction = captureScalar
	castleFunction = castleScalar
	evalFunction = evalScalar

	if !cpu.X86.HasAVX2 {
		return
	}

	switch NNUEHiddenSize {
	case 64:
		moveFunction = moveAVX2_64
		captureFunction = captureAVX2_64
		castleFunction = castleAVX2_64
		evalFunction = getEvalAVX2_64

	case 128:
		moveFunction = moveAVX2_128
		captureFunction = captureAVX2_128
		castleFunction = castleAVX2_128
		evalFunction = getEvalAVX2_128

	case 256:
		moveFunction = moveAVX2_256
		captureFunction = captureAVX2_256
		castleFunction = castleAVX2_256
		evalFunction = getEvalAVX2_256

	case 512:
		moveFunction = moveAVX2_512
		captureFunction = captureAVX2_512
		castleFunction = castleAVX2_512
		evalFunction = getEvalAVX2_512

	default:
		panic("unsupported NNUE hidden size")
	}
}

// Clear = empty-board state = biases only.
func (acc *Accumulator) clear() {
	a0 := &acc.values[0]
	a1 := &acc.values[1]
	biases := &nnueParams.InputBiases

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] = biases[i]
		a1[i] = biases[i]
	}
}

// Copy the accumulator
func (acc *Accumulator) copyFrom(src *Accumulator) {
	*acc = *src
}

// Add one feature: piece(color,type) on sq.
func (acc *Accumulator) addPiece(color, pt, sq int) {
	idx0 := color*384 + pt*64 + sq
	idx1 := (color^1)*384 + pt*64 + (sq ^ 56)

	a0 := &acc.values[0]
	a1 := &acc.values[1]

	w0 := &nnueParams.InputWeights[idx0]
	w1 := &nnueParams.InputWeights[idx1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += w0[i]
		a1[i] += w1[i]
	}
}

// Remove one feature: piece(color,type) from sq.
func (acc *Accumulator) delPiece(color, pt, sq int) {
	idx0 := color*384 + pt*64 + sq
	idx1 := (color^1)*384 + pt*64 + (sq ^ 56)

	a0 := &acc.values[0]
	a1 := &acc.values[1]

	w0 := &nnueParams.InputWeights[idx0]
	w1 := &nnueParams.InputWeights[idx1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] -= w0[i]
		a1[i] -= w1[i]
	}
}

// Move one piece without a capture. Loops are expensive,
// so we use one instead of separate addition/deletion loops.
func (acc *Accumulator) move(color, pt, from, to int) {
	from0 := color*384 + pt*64 + from
	to0 := color*384 + pt*64 + to

	from1 := (color^1)*384 + pt*64 + (from ^ 56)
	to1 := (color^1)*384 + pt*64 + (to ^ 56)

	moveFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		&nnueParams.InputWeights[from0][0],
		&nnueParams.InputWeights[to0][0],

		&nnueParams.InputWeights[from1][0],
		&nnueParams.InputWeights[to1][0],
	)

}

func (acc *Accumulator) capture(
	moverColor, moverPT, from, to int,
	capturedColor, capturedPT, capturedSq int,
) {
	mFrom0 := moverColor*384 + moverPT*64 + from
	mTo0 := moverColor*384 + moverPT*64 + to
	cap0 := capturedColor*384 + capturedPT*64 + capturedSq

	mFrom1 := (moverColor^1)*384 + moverPT*64 + (from ^ 56)
	mTo1 := (moverColor^1)*384 + moverPT*64 + (to ^ 56)
	cap1 := (capturedColor^1)*384 + capturedPT*64 + (capturedSq ^ 56)

	captureFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		&nnueParams.InputWeights[mTo0][0],
		&nnueParams.InputWeights[mFrom0][0],
		&nnueParams.InputWeights[cap0][0],

		&nnueParams.InputWeights[mTo1][0],
		&nnueParams.InputWeights[mFrom1][0],
		&nnueParams.InputWeights[cap1][0],
	)

}

func (acc *Accumulator) castle(
	color, kingFrom, kingTo, rookFrom, rookTo int,
) {
	kFrom0 := color*384 + K*64 + kingFrom
	kTo0 := color*384 + K*64 + kingTo
	rFrom0 := color*384 + R*64 + rookFrom
	rTo0 := color*384 + R*64 + rookTo

	kFrom1 := (color^1)*384 + K*64 + (kingFrom ^ 56)
	kTo1 := (color^1)*384 + K*64 + (kingTo ^ 56)
	rFrom1 := (color^1)*384 + R*64 + (rookFrom ^ 56)
	rTo1 := (color^1)*384 + R*64 + (rookTo ^ 56)

	castleFunction(
		&acc.values[0][0],
		&acc.values[1][0],

		&nnueParams.InputWeights[kFrom0][0],
		&nnueParams.InputWeights[kTo0][0],
		&nnueParams.InputWeights[rFrom0][0],
		&nnueParams.InputWeights[rTo0][0],

		&nnueParams.InputWeights[kFrom1][0],
		&nnueParams.InputWeights[kTo1][0],
		&nnueParams.InputWeights[rFrom1][0],
		&nnueParams.InputWeights[rTo1][0],
	)
}

func (acc *Accumulator) promotion(color, from, to, prom int) {
	from0 := color*384 + P*64 + from
	to0 := color*384 + prom*64 + to

	from1 := (color^1)*384 + P*64 + (from ^ 56)
	to1 := (color^1)*384 + prom*64 + (to ^ 56)

	a0 := &acc.values[0]
	a1 := &acc.values[1]

	wFrom0 := &nnueParams.InputWeights[from0]
	wTo0 := &nnueParams.InputWeights[to0]
	wFrom1 := &nnueParams.InputWeights[from1]
	wTo1 := &nnueParams.InputWeights[to1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wTo0[i] - wFrom0[i]
		a1[i] += wTo1[i] - wFrom1[i]
	}
}

func (acc *Accumulator) promotionCapture(color, from, to, prom, captType int) {
	enemy := color ^ 1

	from0 := color*384 + P*64 + from
	to0 := color*384 + prom*64 + to
	cap0 := enemy*384 + captType*64 + to

	from1 := enemy*384 + P*64 + (from ^ 56)
	to1 := enemy*384 + prom*64 + (to ^ 56)
	cap1 := color*384 + captType*64 + (to ^ 56)

	a0 := &acc.values[0]
	a1 := &acc.values[1]

	wFrom0 := &nnueParams.InputWeights[from0]
	wTo0 := &nnueParams.InputWeights[to0]
	wCap0 := &nnueParams.InputWeights[cap0]

	wFrom1 := &nnueParams.InputWeights[from1]
	wTo1 := &nnueParams.InputWeights[to1]
	wCap1 := &nnueParams.InputWeights[cap1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wTo0[i] - wFrom0[i] - wCap0[i]
		a1[i] += wTo1[i] - wFrom1[i] - wCap1[i]
	}
}

// apply full nnue accumulator update
func (acc *Accumulator) applyPendingChanges(u *Update) {

	// already applied
	if !u.dirty {
		return
	}

	switch u.flag {
	case uNORMAL, uEP_SET:
		acc.move(u.color, u.movingType, u.from, u.to)

	case uCAPTURE:
		acc.capture(u.color, u.movingType, u.from, u.to, u.color^1, u.captType, u.capSq)

	case uEP_CAP:
		acc.capture(u.color, P, u.from, u.to, u.color^1, P, u.capSq)

	case uCASTLE:
		acc.castle(u.color, u.from, u.to, u.rookFrom, u.rookTo)

	case uPROMO:
		acc.promotion(u.color, u.from, u.to, u.prom)

	case uPROMCAPT:
		acc.promotionCapture(u.color, u.from, u.to, u.prom, u.captType)

	}

	u.dirty = false
}

// Rebuild the accumulator from the current board.
func refresh(p *Pos, acc *Accumulator) {
	acc.clear()

	for sq := 0; sq < 64; sq++ {
		piece := p.board[sq]
		if piece == NO_PC {
			continue
		}

		color := colorOf(piece)
		pt := typeOf(piece)
		acc.addPiece(color, pt, sq)
	}
}

// Squared clipped ReLu
func screluWeighted(x, w int16) int32 {
	v := int32(x)

	if v < 0 {
		v = 0
	} else if v > NNUEL0Scale {
		v = NNUEL0Scale
	}

	return v * v * int32(w)
}

func (acc *Accumulator) getEval(stm int) int {
	var sum int32

	evalFunction(
		&acc.values[stm][0],
		&acc.values[stm^1][0],
		&nnueParams.OutputWeights[0][0],
		&nnueParams.OutputWeights[1][0],
		&sum,
	)

	sum = sum/NNUEL0Scale + int32(nnueParams.OutputBias)

	return int(sum * NNUEEvalScale /
		(NNUEL0Scale * NNUEL1Scale))
}

func nnueInitEmbedded() bool {
	need := int(unsafe.Sizeof(NNUEParameters{}))
	if len(embeddedNet) < need {
		return false
	}

	base := unsafe.Pointer(&embeddedNet[0])
	if uintptr(base)%unsafe.Alignof(NNUEParameters{}) != 0 {
		panic("embedded net is misaligned")
	}

	nnueParams = (*NNUEParameters)(base)
	nnue.Loaded = true
	return true
}

// Load a raw Bullet-compatible parameter blob.
func nnueLoad(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		nnue.Loaded = false
		return false
	}

	offset := 0

	nnueParams = new(NNUEParameters)

	readI16 := func() int16 {
		value := int16(
			uint16(data[offset]) |
				uint16(data[offset+1])<<8,
		)

		offset += 2
		return value
	}

	for input := 0; input < NNUEInputSize; input++ {
		for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
			nnueParams.InputWeights[input][neuron] = readI16()
		}
	}

	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		nnueParams.InputBiases[neuron] = readI16()
	}

	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		nnueParams.OutputWeights[0][neuron] = readI16()
	}

	for neuron := 0; neuron < NNUEHiddenSize; neuron++ {
		nnueParams.OutputWeights[1][neuron] = readI16()
	}

	nnueParams.OutputBias = readI16()
	nnue.Loaded = true

	return true
}
