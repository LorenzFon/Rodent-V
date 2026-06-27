package main

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"
)

var kConst = 1.335

// Full loaded datasets.
var allEpd10 []string
var allEpd01 []string
var allEpd05 []string

// Working sampled datasets used by texelFit().
var epd10 []string
var epd01 []string
var epd05 []string

// Usage counters for weighted sampling.
var used10 []int
var used01 []int
var used05 []int

var tunerLoaded bool

func loadTunerFile() {
	if tunerLoaded {
		return
	}

	rand.Seed(time.Now().UnixNano())

	epdFile, err := os.Open("c:/lichess-quiet.epd")
	fmt.Printf("reading epdFile 'c:/lichess-quiet.epd' (%v)\n", err == nil)
	if err != nil {
		fmt.Println("Epd file not found!")
		return
	}
	defer epdFile.Close()

	scanner := bufio.NewScanner(epdFile)
	readCnt := 0

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		readCnt++

		if readCnt%1000000 == 0 {
			fmt.Printf("%d positions loaded\n", readCnt)
		}

		// all epd array
		epdLines = append(epdLines, line)

		// per-score arrays
		switch {
		case strings.Contains(line, "1/2-1/2"):
			allEpd05 = append(allEpd05, line)
		case strings.Contains(line, "1-0"):
			allEpd10 = append(allEpd10, line)
		case strings.Contains(line, "0-1"):
			allEpd01 = append(allEpd01, line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("error while reading epd file: %v\n", err)
	}

	used10 = make([]int, len(allEpd10))
	used01 = make([]int, len(allEpd01))
	used05 = make([]int, len(allEpd05))

	tunerLoaded = true

	fmt.Printf("%d Total positions loaded\n", readCnt)
	fmt.Printf("W: %d  L: %d  D: %d\n", len(allEpd10), len(allEpd01), len(allEpd05))
}

// Picks n distinct indices, preferring positions used less often.
func pickWeightedIndices(used []int, n int) []int {
	if len(used) == 0 || n <= 0 {
		return nil
	}

	if n >= len(used) {
		idx := make([]int, len(used))
		for i := range used {
			idx[i] = i
			used[i]++
		}
		return idx
	}

	chosen := make([]int, 0, n)
	picked := make([]bool, len(used))

	for len(chosen) < n {
		totalWeight := 0.0
		for i := range used {
			if picked[i] {
				continue
			}
			totalWeight += 1.0 / float64(1+used[i])
		}

		r := rand.Float64() * totalWeight
		acc := 0.0

		for i := range used {
			if picked[i] {
				continue
			}
			acc += 1.0 / float64(1+used[i])
			if acc >= r {
				picked[i] = true
				used[i]++
				chosen = append(chosen, i)
				break
			}
		}
	}

	return chosen
}

func samplePositions(all []string, used []int, n int) []string {
	indices := pickWeightedIndices(used, n)
	out := make([]string, 0, len(indices))
	for _, idx := range indices {
		out = append(out, all[idx])
	}
	return out
}

// Builds a fresh working sample for Texel fitting.
// samplePerBucket means:
//
//	samplePerBucket positions from 1-0
//	samplePerBucket positions from 0-1
//	samplePerBucket positions from 1/2-1/2
func tunerInitBatch(samplePerBucket int) {
	loadTunerFile()
	if !tunerLoaded {
		return
	}

	epd10 = samplePositions(allEpd10, used10, samplePerBucket)
	epd01 = samplePositions(allEpd01, used01, samplePerBucket)
	epd05 = samplePositions(allEpd05, used05, samplePerBucket)

	fmt.Printf("sampled: W %d  L %d  D %d\n", len(epd10), len(epd01), len(epd05))
}

// tunerInitBatchAll loads all the positions from file.
// Used in getFit to get mean square error on the entire training set.
func tunerInitBatchAll() {
	loadTunerFile()
	if !tunerLoaded {
		return
	}

	epd10 = allEpd10
	epd01 = allEpd01
	epd05 = allEpd05

	fmt.Printf("loaded: W %d  L %d  D %d\n", len(epd10), len(epd01), len(epd05))
}

// texelSigmoid translates eval score into a winning probability
// between 0 and 1.
func texelSigmoid(score int, k float64) float64 {
	exponent := -(k * float64(score) / 400.0)
	return 1.0 / (1.0 + math.Pow(10.0, exponent))
}

// texelFit averages the difference between texelSigmoid and actual
// game result for multpile positions. Lower error means that our
// evaluation function predicts game result more accurately.
func texelFit() float64 {
	var p Pos
	sum := 0.0
	iteration := 0

	result := 1.0
	for i := 0; i < len(epd10); i++ {
		iteration++
		parseFEN(&p, epd10[i])

		score := eval_internal(&p, false)
		if p.side == Black {
			score = -score
		}

		sigmoid := texelSigmoid(score, kConst)
		diff := result - sigmoid
		sum += diff * diff
	}

	result = 0.0
	for i := 0; i < len(epd01); i++ {
		iteration++
		parseFEN(&p, epd01[i])

		score := eval_internal(&p, false)
		if p.side == Black {
			score = -score
		}

		sigmoid := texelSigmoid(score, kConst)
		diff := result - sigmoid
		sum += diff * diff
	}

	result = 0.5
	for i := 0; i < len(epd05); i++ {
		iteration++
		parseFEN(&p, epd05[i])

		score := eval_internal(&p, false)
		if p.side == Black {
			score = -score
		}

		sigmoid := texelSigmoid(score, kConst)
		diff := result - sigmoid
		sum += diff * diff
	}

	if iteration == 0 {
		return 0.0
	}

	return 1000.0 * (sum / float64(iteration))
}

// getFit calculates mean square error based on the entire dataset.
// Useful mainly for verification of a tuner run or for manual
// optimization of values.
func getFit() {
	loadTunerFile()
	tunerInitBatchAll()
	var bestFit = texelFit()
	fmt.Print(bestFit)
}

func centerStats() {
	loadTunerFile()
	tunerInitBatchAll()
	iteration := 0
	kid := 0
	french := 0
	sicilian := 0
	e4e5 := 0
	d4d5 := 0
	var p Pos
	var e EvalData

	for i := 0; i < len(epdLines); i++ {
		iteration++
		parseFEN(&p, epdLines[i])
		initCenterType(&p, &e)
		if e.center[White] == KID_high {
			kid++
		}
		if e.center[White] == KID_low {
			kid++
		}
		if e.center[White] == FRENCH_low {
			french++
		}
		if e.center[White] == FRENCH_high {
			french++
		}
		if e.center[White] == SICILIAN_high {
			sicilian++
		}
		if e.center[White] == SICILIAN_low {
			sicilian++
		}
		if e.center[White] == CLASSIC_e4e5 {
			e4e5++
		}
		if e.center[White] == CLASSIC_d4d5 {
			d4d5++
		}
	}

	sum := sicilian + french + kid + e4e5 + d4d5
	fmt.Print(iteration, " positions, ", sum, " known central pawn structures - ", sum*100/iteration, " percent\n")
	fmt.Print(sicilian, " sicilian, ", kid, " kid, ", french, " french, ", e4e5, " e4e5, ", d4d5, " d4d5 ")
}

// tuneValue is a helper function that tries to modify a single value
func tuneValue(orig int, bestFit float64, set func(int)) (int, float64, bool) {
	localBestVal := orig
	localBestFit := bestFit

	// try adding 1 to the current weight
	firstTrySucceeded := false
	set(orig + 1)
	fitPlus := texelFit()
	if fitPlus < localBestFit {
		firstTrySucceeded = true
		localBestFit = fitPlus
		localBestVal = orig + 1
	}

	// if addition failed, try subtracting
	if !firstTrySucceeded {
		set(orig - 1)
		fitMinus := texelFit()
		if fitMinus < localBestFit {
			localBestFit = fitMinus
			localBestVal = orig - 1
		}
	}

	set(localBestVal)
	return localBestVal, localBestFit, localBestVal != orig
}

func tunerFree() {

	epdLines = nil

	epd10 = nil
	epd01 = nil
	epd05 = nil

	allEpd10 = nil
	allEpd01 = nil
	allEpd05 = nil

	used10 = nil
	used01 = nil
	used05 = nil

	tunerLoaded = false
}

// tunePST is a classical Texel hill-climber for one PST table set.
// It calls tunerInitBatch() at the beginning, to select
// a new batch of positions.
func tunePST(table *[6][64]int, tableName string, samplePerBucket int) {
	tunerInitBatch(samplePerBucket)

	bestFit := texelFit()
	fmt.Printf("Starting new batch for %s, initial fit = %.6f\n", tableName, bestFit)

	for piece := 0; piece < 6; piece++ {
		for sq := 0; sq < 64; sq++ {
			orig := table[piece][sq]

			localBestVal, localBestFit, changed := tuneValue(orig, bestFit, func(v int) {
				table[piece][sq] = v
			})

			if changed {
				bestFit = localBestFit
				fmt.Printf("%s[%d][%d]: %d -> %d  fit = %.6f\n",
					tableName, piece, sq, orig, localBestVal, bestFit)
			}
		}
	}

	fmt.Printf("Finished batch %s, final fit = %.6f\n", tableName, bestFit)
}

var pstLabels = [6]string{"P", "N", "B", "R", "Q", "K"}
var pstPieceName = [6]string{"pawn", "knight", "bishop", "rook", "queen", "king"}

// prints one-dimensional table; var table can be a slice,
// which allows to print different-sized tables using one function
func print1DTable(name string, table []int) {
	fmt.Printf("var %s = [%d]int{", name, len(table))

	for i, v := range table {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%d", v)
	}

	fmt.Println("}")
}

// prints two-dimensional table, assuming first dimenson is [2].
// uses a slice to allows printing different-sized tables with one function
func print2DTable(name string, table [][]int) {
	fmt.Printf("var %s = [2][%d]int{\n", name, len(table[0]))

	for row := 0; row < 2; row++ {
		fmt.Printf("\t%d: {", row)
		for col, v := range table[row] {
			if col > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%d", v)
		}
		fmt.Println("},")
	}

	fmt.Println("}")
}

// prints a set of piece/square tables for all the pieces
func printPSTforAllPieces(name string, pst *[6][64]int) {
	fmt.Printf("var %s = [6][64]int{\n", name)

	for piece := 0; piece < 6; piece++ {
		fmt.Printf("\t%s: {\n", pstLabels[piece])

		for row := 0; row < 4; row++ {
			start := row * 16

			fmt.Print("\t\t")
			for i := 0; i < 16; i++ {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("%d", pst[piece][start+i])
			}

			switch row {
			case 0:
				fmt.Printf(", /* %-6s r1-r2 */\n", pstPieceName[piece])
			case 1:
				fmt.Printf(", /*        r3-r4 */\n")
			case 2:
				fmt.Printf(", /*        r5-r6 */\n")
			case 3:
				fmt.Printf(",\n")
			}
		}

		fmt.Println("\t},")
	}

	fmt.Println("}")
}

func printPSTSet() {
	printPSTforAllPieces("pstMG", &pstMG)
	fmt.Println()
	printPSTforAllPieces("pstEG", &pstEG)
}

//--- SIMPLE ARRAY TUNERS --- //

func tunePerPiece1D(table *[6]int, tableName string, samplePerBucket int) {
	tunerInitBatch(samplePerBucket)

	bestFit := texelFit()
	fmt.Printf("Starting new batch for %s, initial fit = %.6f\n", tableName, bestFit)

	for piece := 0; piece < 5; piece++ { // skip king slot
		orig := table[piece]

		localBestVal, localBestFit, changed := tuneValue(orig, bestFit, func(v int) {
			table[piece] = v
		})

		if changed {
			bestFit = localBestFit
			fmt.Printf("%s[%d]: %d -> %d  fit = %.6f\n",
				tableName, piece, orig, localBestVal, bestFit)
		}
	}

	fmt.Printf("Finished batch %s, final fit = %.6f\n", tableName, bestFit)
}

// tunePerRank2D tunes a [2][8]int table in the same style as tunePST.
// Typical use: passedBonusMG / passedBonusEG.
func tunePerRank2D(table *[2][8]int, tableName string, samplePerBucket int) {
	tunerInitBatch(samplePerBucket)

	bestFit := texelFit()
	fmt.Printf("Starting new batch for %s, initial fit = %.6f\n", tableName, bestFit)

	for row := 0; row < 2; row++ {
		for rank := 0; rank < 8; rank++ {
			orig := table[row][rank]

			localBestVal, localBestFit, changed := tuneValue(orig, bestFit, func(v int) {
				table[row][rank] = v
			})

			if changed {
				bestFit = localBestFit
				fmt.Printf("%s[%d][%d]: %d -> %d  fit = %.6f\n",
					tableName, row, rank, orig, localBestVal, bestFit)
			}

		}
	}

	fmt.Printf("Finished batch %s, final fit = %.6f\n", tableName, bestFit)
}

// tunePerRank1D tunes a [8]int table in the same style as tunePST.
// Typical use: ourPasserProximityMG/EG, theirPasserProximityMG/EG.
func tunePerRank1D(table *[8]int, tableName string, samplePerBucket int) {
	tunerInitBatch(samplePerBucket)

	bestFit := texelFit()
	fmt.Printf("Starting new batch for %s, initial fit = %.6f\n", tableName, bestFit)

	for i := 0; i < 8; i++ {
		orig := table[i]

		localBestVal, localBestFit, changed := tuneValue(orig, bestFit, func(v int) {
			table[i] = v
		})

		if changed {
			bestFit = localBestFit
			fmt.Printf("%s[%d]: %d -> %d  fit = %.6f\n",
				tableName, i, orig, localBestVal, bestFit)
		}
	}

	fmt.Printf("Finished batch %s, final fit = %.6f\n", tableName, bestFit)
}

func tunePerPiece2D(table *[2][6]int, tableName string, samplePerBucket int) {
	tunerInitBatch(samplePerBucket)

	bestFit := texelFit()
	fmt.Printf("Starting new batch for %s, initial fit = %.6f\n", tableName, bestFit)

	for row := 0; row < 2; row++ {
		for piece := 0; piece < 5; piece++ { // skip king entry
			orig := table[row][piece]

			localBestVal, localBestFit, changed := tuneValue(orig, bestFit, func(v int) {
				table[row][piece] = v
			})

			if changed {
				bestFit = localBestFit
				fmt.Printf("%s[%d][%d]: %d -> %d  fit = %.6f\n",
					tableName, row, piece, orig, localBestVal, bestFit)
			}
		}
	}

	fmt.Printf("Finished batch %s, final fit = %.6f\n", tableName, bestFit)
}

func pstTuningSession() {
	for i := 0; i < 100; i++ {
		tunePST(&pstMG, "pstMG", 5000)
		tunePST(&pstEG, "pstEG", 5000)
		printPSTforAllPieces("pstMG", &pstMG)
		printPSTforAllPieces("pstEG", &pstEG)
	}
}

func passerTuningSession() {
	for i := 0; i < 100; i++ {
		tunePerRank2D(&passedBonusMG, "passedBonusMG", 5000)
		tunePerRank2D(&passedBonusEG, "passedBonusEG", 5000)

		tunePerRank1D(&ourPasserProximityMG, "ourPasserProximityMG", 5000)
		tunePerRank1D(&ourPasserProximityEG, "ourPasserProximityEG", 5000)
		tunePerRank1D(&theirPasserProximityMG, "theirPasserProximityMG", 5000)
		tunePerRank1D(&theirPasserProximityEG, "theirPasserProximityEG", 5000)

		print2DTable("passedBonusMG", [][]int{passedBonusMG[0][:], passedBonusMG[1][:]})
		print2DTable("passedBonusEG", [][]int{passedBonusEG[0][:], passedBonusEG[1][:]})

		print1DTable("ourPasserProximityMG", ourPasserProximityMG[:])
		print1DTable("ourPasserProximityEG", ourPasserProximityEG[:])
		print1DTable("theirPasserProximityMG", theirPasserProximityMG[:])
		print1DTable("theirPasserProximityEG", theirPasserProximityEG[:])
	}
}

func threatTuningSession() {
	for i := 0; i < 100; i++ {
		fmt.Println("STAGE ", i)
		tunePerPiece1D(&threatByPawnMG, "threatByPawnMG", 5000)
		tunePerPiece1D(&threatByPawnEG, "threatByPawnEG", 5000)

		tunePerPiece2D(&threatByKnightMG, "threatByKnightMG", 5000)
		tunePerPiece2D(&threatByKnightEG, "threatByKnightEG", 5000)

		tunePerPiece2D(&threatByBishopMG, "threatByBishopMG", 5000)
		tunePerPiece2D(&threatByBishopEG, "threatByBishopEG", 5000)

		tunePerPiece2D(&threatByRookMG, "threatByRookMG", 5000)
		tunePerPiece2D(&threatByRookEG, "threatByRookEG", 5000)

		tunePerPiece2D(&threatByQueenMG, "threatByQueenMG", 5000)
		tunePerPiece2D(&threatByQueenEG, "threatByQueenEG", 5000)

		tunePerPiece1D(&threatByKingMG, "threatByKingMG", 5000)
		tunePerPiece1D(&threatByKingEG, "threatByKingEG", 5000)

		print1DTable("threatByPawnMG", threatByPawnMG[:])
		print1DTable("threatByPawnEG", threatByPawnEG[:])

		print2DTable("threatByKnightMG", [][]int{threatByKnightMG[0][:], threatByKnightMG[1][:]})
		print2DTable("threatByKnightEG", [][]int{threatByKnightEG[0][:], threatByKnightEG[1][:]})

		print2DTable("threatByBishopMG", [][]int{threatByBishopMG[0][:], threatByBishopMG[1][:]})
		print2DTable("threatByBishopEG", [][]int{threatByBishopEG[0][:], threatByBishopEG[1][:]})

		print2DTable("threatByRookMG", [][]int{threatByRookMG[0][:], threatByRookMG[1][:]})
		print2DTable("threatByRookEG", [][]int{threatByRookEG[0][:], threatByRookEG[1][:]})

		print2DTable("threatByQueenMG", [][]int{threatByQueenMG[0][:], threatByQueenMG[1][:]})
		print2DTable("threatByQueenEG", [][]int{threatByQueenEG[0][:], threatByQueenEG[1][:]})

		print1DTable("threatByKingMG", threatByKingMG[:])
		print1DTable("threatByKingEG", threatByKingEG[:])
	}
}
