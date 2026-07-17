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
