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

// The accumulator itself now belongs to Pos.
// This state contains only information shared by the engine.
type NNUEState struct {
	Loaded bool
}

var nnueParams NNUEParameters
var nnue NNUEState

// Add this field to Pos:
//
// nnueAccumulator [2][NNUEHiddenSize]int16

// Clear = empty-board state = biases only.
func nnueClear(p *Pos) {
	a0 := &p.nnueAccumulator[0]
	a1 := &p.nnueAccumulator[1]
	biases := &nnueParams.InputBiases

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] = biases[i]
		a1[i] = biases[i]
	}
}

// Add one feature: piece(color,type) on sq.
func nnueAddPiece(p *Pos, color, pt, sq int) {
	idx0 := color*384 + pt*64 + sq
	idx1 := (color^1)*384 + pt*64 + (sq ^ 56)

	a0 := &p.nnueAccumulator[0]
	a1 := &p.nnueAccumulator[1]

	w0 := &nnueParams.InputWeights[idx0]
	w1 := &nnueParams.InputWeights[idx1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += w0[i]
		a1[i] += w1[i]
	}
}

// Remove one feature: piece(color,type) from sq.
func nnueDelPiece(p *Pos, color, pt, sq int) {
	idx0 := color*384 + pt*64 + sq
	idx1 := (color^1)*384 + pt*64 + (sq ^ 56)

	a0 := &p.nnueAccumulator[0]
	a1 := &p.nnueAccumulator[1]

	w0 := &nnueParams.InputWeights[idx0]
	w1 := &nnueParams.InputWeights[idx1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] -= w0[i]
		a1[i] -= w1[i]
	}
}

// Move one piece without a capture.
func nnueMovePiece(p *Pos, color, pt, from, to int) {
	from0 := color*384 + pt*64 + from
	to0 := color*384 + pt*64 + to

	from1 := (color^1)*384 + pt*64 + (from ^ 56)
	to1 := (color^1)*384 + pt*64 + (to ^ 56)

	a0 := &p.nnueAccumulator[0]
	a1 := &p.nnueAccumulator[1]

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
//
// capturedSq is normally equal to to, except for en passant.
func nnueMoveCapture(
	p *Pos,
	moverColor, moverPT, from, to int,
	capturedColor, capturedPT, capturedSq int,
) {
	mFrom0 := moverColor*384 + moverPT*64 + from
	mTo0 := moverColor*384 + moverPT*64 + to
	cap0 := capturedColor*384 + capturedPT*64 + capturedSq

	mFrom1 := (moverColor^1)*384 + moverPT*64 + (from ^ 56)
	mTo1 := (moverColor^1)*384 + moverPT*64 + (to ^ 56)
	cap1 := (capturedColor^1)*384 + capturedPT*64 + (capturedSq ^ 56)

	a0 := &p.nnueAccumulator[0]
	a1 := &p.nnueAccumulator[1]

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

// Unmake a capture.
//
// The accumulator is assumed to represent the position after the capture.
// This moves the capturing piece back and restores the captured piece.
func nnueMoveUncapture(
	p *Pos,
	moverColor, moverPT, from, to int,
	capturedColor, capturedPT, capturedSq int,
) {
	mFrom0 := moverColor*384 + moverPT*64 + from
	mTo0 := moverColor*384 + moverPT*64 + to
	cap0 := capturedColor*384 + capturedPT*64 + capturedSq

	mFrom1 := (moverColor^1)*384 + moverPT*64 + (from ^ 56)
	mTo1 := (moverColor^1)*384 + moverPT*64 + (to ^ 56)
	cap1 := (capturedColor^1)*384 + capturedPT*64 + (capturedSq ^ 56)

	a0 := &p.nnueAccumulator[0]
	a1 := &p.nnueAccumulator[1]

	wMFrom0 := &nnueParams.InputWeights[mFrom0]
	wMTo0 := &nnueParams.InputWeights[mTo0]
	wCap0 := &nnueParams.InputWeights[cap0]

	wMFrom1 := &nnueParams.InputWeights[mFrom1]
	wMTo1 := &nnueParams.InputWeights[mTo1]
	wCap1 := &nnueParams.InputWeights[cap1]

	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wMFrom0[i] - wMTo0[i] + wCap0[i]
		a1[i] += wMFrom1[i] - wMTo1[i] + wCap1[i]
	}
}

// Rebuild the accumulator from the current board.
func nnueRefresh(p *Pos) {
	nnueClear(p)

	for sq := 0; sq < 64; sq++ {
		piece := p.board[sq]
		if piece == NO_PC {
			continue
		}

		color := colorOf(piece)
		pt := typeOf(piece)

		nnueAddPiece(p, color, pt, sq)
	}
}

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
func nnueEvaluate(p *Pos) int {
	stm := p.side

	a0 := &p.nnueAccumulator[stm]
	a1 := &p.nnueAccumulator[stm^1]

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
