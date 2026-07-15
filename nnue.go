package main

import (
	"os"
)

const (
	NNUEInputSize  = 768
	NNUEHiddenSize = 64
	NNUEEvalScale  = 400
	NNUEL0Scale    = 255
	NNUEL1Scale    = 64
)

type NNUEParameters struct {
	InputWeights  [NNUEInputSize][NNUEHiddenSize]int16
	InputBiases   [NNUEHiddenSize]int16
	OutputWeights [2][NNUEHiddenSize]int16
	OutputBias    int16
}

var nnueParams NNUEParameters
var nnue NNUEState

type Accumulator struct {
	values [2][NNUEHiddenSize]int16
}

// The accumulator itself now belongs to searchState.
// This state contains only information shared by the engine.
type NNUEState struct {
	Loaded bool
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

// Change piece type on the same square.
// Useful for promotion: pawn -> knight/bishop/rook/queen.
func (acc *Accumulator) changePiece(color, oldPT, newPT, sq int) {
	old0 := color*384 + oldPT*64 + sq
	new0 := color*384 + newPT*64 + sq

	old1 := (color^1)*384 + oldPT*64 + (sq ^ 56)
	new1 := (color^1)*384 + newPT*64 + (sq ^ 56)

	a0 := &acc.values[0]
	a1 := &acc.values[1]

	wOld0 := &nnueParams.InputWeights[old0]
	wNew0 := &nnueParams.InputWeights[new0]

	wOld1 := &nnueParams.InputWeights[old1]
	wNew1 := &nnueParams.InputWeights[new1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wNew0[i] - wOld0[i]
		a1[i] += wNew1[i] - wOld1[i]
	}
}

// Move one piece without a capture. Loops are expensive,
// so we use one instead of separate addition/deletion loops.
func (acc *Accumulator) movePiece(color, pt, from, to int) {
	from0 := color*384 + pt*64 + from
	to0 := color*384 + pt*64 + to

	from1 := (color^1)*384 + pt*64 + (from ^ 56)
	to1 := (color^1)*384 + pt*64 + (to ^ 56)

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

// Make a capture.
// Here we perform 3 actions in a loop.
// capturedSq is normally same as to, except for en passant.
func (acc *Accumulator) moveAndCapture(
	moverColor, moverPT, from, to int,
	capturedColor, capturedPT, capturedSq int) {
	mFrom0 := moverColor*384 + moverPT*64 + from
	mTo0 := moverColor*384 + moverPT*64 + to
	cap0 := capturedColor*384 + capturedPT*64 + capturedSq

	mFrom1 := (moverColor^1)*384 + moverPT*64 + (from ^ 56)
	mTo1 := (moverColor^1)*384 + moverPT*64 + (to ^ 56)
	cap1 := (capturedColor^1)*384 + capturedPT*64 + (capturedSq ^ 56)

	a0 := &acc.values[0]
	a1 := &acc.values[1]

	wMFrom0 := &nnueParams.InputWeights[mFrom0]
	wMTo0 := &nnueParams.InputWeights[mTo0]
	wCap0 := &nnueParams.InputWeights[cap0]

	wMFrom1 := &nnueParams.InputWeights[mFrom1]
	wMTo1 := &nnueParams.InputWeights[mTo1]
	wCap1 := &nnueParams.InputWeights[cap1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wMTo0[i] - wMFrom0[i] - wCap0[i]
		a1[i] += wMTo1[i] - wMFrom1[i] - wCap1[i]
	}
}

// apply full nnue accumulator update
func (acc *Accumulator) applyPendingChanges(u *Update) {

	// already applied
	if !u.dirty {
		return
	}

	switch u.flag {
	case NORMAL, EP_SET:
		{
			if u.captType != NO_TP {
				acc.moveAndCapture(u.color, u.movingType, u.from, u.to, u.color^1, u.captType, u.capSq)
			} else {
				acc.movePiece(u.color, u.movingType, u.from, u.to)
			}
		}

	case EP_CAP:
		acc.moveAndCapture(u.color, P, u.from, u.to, u.color^1, P, u.capSq)

	case CASTLE:
		acc.movePiece(u.color, K, u.from, u.to)
		acc.movePiece(u.color, R, u.rookFrom, u.rookTo)

	case N_PROM, B_PROM, R_PROM, Q_PROM:
		// Move pawn from source to destination first.
		acc.movePiece(u.color, P, u.from, u.to)

		// Capture
		if u.captType != NO_TP {
			acc.delPiece(u.color^1, u.captType, u.to)
		}

		// Replace pawn with promoted piece on destination.
		acc.changePiece(u.color, P, u.prom, u.to)

	}

	u.dirty = false
}

// Rebuild the accumulator from the current board.
func (acc *Accumulator) refresh(p *Pos) {
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

// Return the score from the perspective of the side to move.
func nnueEvaluate(p *Pos, acc *Accumulator) int {
	stm := p.side

	a0 := &acc.values[stm]
	a1 := &acc.values[stm^1]

	w0 := &nnueParams.OutputWeights[0]
	w1 := &nnueParams.OutputWeights[1]

	var sum0 int32
	var sum1 int32
	var sum2 int32
	var sum3 int32

	for i := 0; i < NNUEHiddenSize; i += 4 {
		sum0 += screluWeighted(a0[i+0], w0[i+0]) +
			screluWeighted(a1[i+0], w1[i+0])

		sum1 += screluWeighted(a0[i+1], w0[i+1]) +
			screluWeighted(a1[i+1], w1[i+1])

		sum2 += screluWeighted(a0[i+2], w0[i+2]) +
			screluWeighted(a1[i+2], w1[i+2])

		sum3 += screluWeighted(a0[i+3], w0[i+3]) +
			screluWeighted(a1[i+3], w1[i+3])
	}

	sum := sum0 + sum1 + sum2 + sum3
	sum = sum/NNUEL0Scale + int32(nnueParams.OutputBias)

	return int(sum * NNUEEvalScale /
		(NNUEL0Scale * NNUEL1Scale))
}

// Load a raw Bullet-compatible parameter blob.
func nnueLoad(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		nnue.Loaded = false
		return false
	}

	offset := 0

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
