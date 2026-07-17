package main

func moveScalar(
	a0, a1 *[NNUEHiddenSize]int16,
	wFrom0, wTo0 *[NNUEHiddenSize]int16,
	wFrom1, wTo1 *[NNUEHiddenSize]int16,
) {
	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wTo0[i] - wFrom0[i]
		a1[i] += wTo1[i] - wFrom1[i]
	}
}

func captureScalar(
	a0, a1 *[NNUEHiddenSize]int16,
	wTo0, wFrom0, wCap0 *[NNUEHiddenSize]int16,
	wTo1, wFrom1, wCap1 *[NNUEHiddenSize]int16,
) {
	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wTo0[i] - wFrom0[i] - wCap0[i]
		a1[i] += wTo1[i] - wFrom1[i] - wCap1[i]
	}
}

func castleScalar(
	a0, a1 *[NNUEHiddenSize]int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *[NNUEHiddenSize]int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *[NNUEHiddenSize]int16,
) {
	for i := 0; i < NNUEHiddenSize; i++ {
		a0[i] += wKTo0[i] - wKFrom0[i] +
			wRTo0[i] - wRFrom0[i]

		a1[i] += wKTo1[i] - wKFrom1[i] +
			wRTo1[i] - wRFrom1[i]
	}
}

// Return the score from the perspective of the side to move.
func (acc *Accumulator) getEvalScalar(stm int) int {

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