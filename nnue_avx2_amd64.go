//go:build amd64

package main

//go:noescape
func captureAVX2(
	a0, a1 *[NNUEHiddenSize]int16,
	wTo0, wFrom0, wCap0 *[NNUEHiddenSize]int16,
	wTo1, wFrom1, wCap1 *[NNUEHiddenSize]int16,
)

//go:noescape
func moveAVX2(
	a0, a1 *[NNUEHiddenSize]int16,
	wFrom0, wTo0 *[NNUEHiddenSize]int16,
	wFrom1, wTo1 *[NNUEHiddenSize]int16,
)

//go:noescape
func castleAVX2(
	a0, a1 *[NNUEHiddenSize]int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *[NNUEHiddenSize]int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *[NNUEHiddenSize]int16,
)

//go:noescape
func promoteAVX2(
	a0, a1 *[NNUEHiddenSize]int16,
	wOld0, wNew0 *[NNUEHiddenSize]int16,
	wOld1, wNew1 *[NNUEHiddenSize]int16,
)