//go:build amd64

package main

// CAPTURE

//go:noescape
func captureAVX2_64(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_128(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_256(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)

//go:noescape
func captureAVX2_512(
	a0, a1 *int16,
	wTo0, wFrom0, wCap0 *int16,
	wTo1, wFrom1, wCap1 *int16,
)



// MOVE

//go:noescape
func moveAVX2_64(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_128(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_256(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

//go:noescape
func moveAVX2_512(
	a0, a1 *int16,
	wFrom0, wTo0 *int16,
	wFrom1, wTo1 *int16,
)

// CASTLE (incomplete)

//go:noescape
func castleAVX2_64(
	a0, a1 *[NNUEHiddenSize]int16,
	wKFrom0, wKTo0, wRFrom0, wRTo0 *[NNUEHiddenSize]int16,
	wKFrom1, wKTo1, wRFrom1, wRTo1 *[NNUEHiddenSize]int16,
)

// EVAL

//go:noescape
func getEvalAVX2_64(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_128(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_256(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)

//go:noescape
func getEvalAVX2_512(
	a0, a1 *int16,
	w0, w1 *int16,
	sum *int32,
)