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

	epdFile, err := os.Open("quiet-labeled.epd")
	fmt.Printf("reading epdFile 'quiet-labeled.epd' (%v)\n", err == nil)
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
//   samplePerBucket positions from 1-0
//   samplePerBucket positions from 0-1
//   samplePerBucket positions from 1/2-1/2
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

		score := evaluate(&p)
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

		score := evaluate(&p)
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

		score := evaluate(&p)
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

func tunerFree() {
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

// tuneOST is a classical Texel hill-climber for one PST table.
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

func printPSTSet() {
	printOnePST("pstMG", &pstMG)
	fmt.Println()
	printOnePST("pstEG", &pstEG)
}

// prints a set of piece/square tables for all the pieces
func printOnePST(name string, pst *[6][64]int) {
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

func printPerRank2D(name string, table *[2][8]int) {
	fmt.Printf("var %s = [2][8]int{\n", name)
	for row := 0; row < 2; row++ {
		comment := "free"
		if row == 1 {
			comment = "blocked"
		}
		fmt.Printf("\t%d: {%d, %d, %d, %d, %d, %d, %d, %d}, // %s\n",
			row,
			table[row][0], table[row][1], table[row][2], table[row][3],
			table[row][4], table[row][5], table[row][6], table[row][7],
			comment,
		)
	}
	fmt.Println("}")
}

func printPerRank1D(name string, table *[8]int) {
	fmt.Printf("var %s = [8]int{%d, %d, %d, %d, %d, %d, %d, %d}\n",
		name,
		table[0], table[1], table[2], table[3],
		table[4], table[5], table[6], table[7],
	)
}

func printPerPiece2D(name string, table *[2][6]int) {
	fmt.Printf("var %s = [2][6]int{\n", name)
	for row := 0; row < 2; row++ {
		fmt.Printf("\t%d: {%d, %d, %d, %d, %d, %d},\n",
			row,
			table[row][0], table[row][1], table[row][2],
			table[row][3], table[row][4], table[row][5],
		)
	}
	fmt.Println("}")
}

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

func printPerPiece1D(name string, table *[6]int) {
	fmt.Printf("var %s = [6]int{%d, %d, %d, %d, %d, %d}\n",
		name,
		table[0], table[1], table[2], table[3], table[4], table[5],
	)
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

func pstTuningSession() {
		for i := 0; i < 100; i++ {
			tunePST(&pstMG, "pstMG", 5000)
			tunePST(&pstEG, "pstEG", 5000)
			printOnePST("pstMG", &pstMG)
			printOnePST("pstEG", &pstEG)
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

		printPerRank2D("passedBonusMG", &passedBonusMG)
		printPerRank2D("passedBonusEG", &passedBonusEG)

		printPerRank1D("ourPasserProximityMG", &ourPasserProximityMG)
		printPerRank1D("ourPasserProximityEG", &ourPasserProximityEG)
		printPerRank1D("theirPasserProximityMG", &theirPasserProximityMG)
		printPerRank1D("theirPasserProximityEG", &theirPasserProximityEG)
	}
}

func threatTuningSession() {
	for i := 0; i < 100; i++ {
		fmt.Println( "STAGE ", i)
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

		printPerPiece1D("threatByPawnMG", &threatByPawnMG)
		printPerPiece1D("threatByPawnEG", &threatByPawnEG)

		printPerPiece2D("threatByKnightMG", &threatByKnightMG)
		printPerPiece2D("threatByKnightEG", &threatByKnightEG)

		printPerPiece2D("threatByBishopMG", &threatByBishopMG)
		printPerPiece2D("threatByBishopEG", &threatByBishopEG)

		printPerPiece2D("threatByRookMG", &threatByRookMG)
		printPerPiece2D("threatByRookEG", &threatByRookEG)

		printPerPiece2D("threatByQueenMG", &threatByQueenMG)
		printPerPiece2D("threatByQueenEG", &threatByQueenEG)

		printPerPiece1D("threatByKingMG", &threatByKingMG)
		printPerPiece1D("threatByKingEG", &threatByKingEG)
	}
}