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

// All epds, regardless of score 
var epdLines[]string

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

func printSinglePST(varName string, label string, table *[64]int) {
	if varName != "" {
		fmt.Printf("var %s = [64]int{\n", varName)
	} else {
		fmt.Printf("\t%s: {\n", label)
	}

	for row := 0; row < 8; row++ {
		start := row * 8

		if varName != "" {
			fmt.Print("\t")
		} else {
			fmt.Print("\t\t")
		}

		for i := 0; i < 8; i++ {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%4d", table[start+i])
		}
		fmt.Println(",")
	}

	if varName != "" {
		fmt.Println("}")
	} else {
		fmt.Println("\t},")
	}
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

		print2DTable("passedBonusMG", [][]int{ passedBonusMG[0][:], passedBonusMG[1][:],})
		print2DTable("passedBonusEG", [][]int{ passedBonusEG[0][:], passedBonusEG[1][:],})

		print1DTable("ourPasserProximityMG", ourPasserProximityMG[:])
		print1DTable("ourPasserProximityEG", ourPasserProximityEG[:])
		print1DTable("theirPasserProximityMG", theirPasserProximityMG[:])
		print1DTable("theirPasserProximityEG", theirPasserProximityEG[:])
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

		print1DTable("threatByPawnMG", threatByPawnMG[:])
		print1DTable("threatByPawnEG", threatByPawnEG[:])

		print2DTable("threatByKnightMG", [][]int{ threatByKnightMG[0][:], threatByKnightMG[1][:],})
		print2DTable("threatByKnightEG", [][]int{ threatByKnightEG[0][:], threatByKnightEG[1][:],})

		print2DTable("threatByBishopMG", [][]int{ threatByBishopMG[0][:], threatByBishopMG[1][:],})
		print2DTable("threatByBishopEG", [][]int{ threatByBishopEG[0][:], threatByBishopEG[1][:],})

		print2DTable("threatByRookMG", [][]int{ threatByRookMG[0][:], threatByRookMG[1][:],})
		print2DTable("threatByRookEG", [][]int{ threatByRookEG[0][:], threatByRookEG[1][:],})

		print2DTable("threatByQueenMG", [][]int{ threatByQueenMG[0][:], threatByQueenMG[1][:],})
		print2DTable("threatByQueenEG", [][]int{ threatByQueenEG[0][:], threatByQueenEG[1][:],})

		print1DTable("threatByKingMG", threatByKingMG[:])
		print1DTable("threatByKingEG", threatByKingEG[:])
	}
}

// Second tuner with data extraction

type PhalanxTunePos struct {
	Result    float64
	BaseScore int
	Phase     int
	Feature   [64]int
}

func ExtractTuneData(epdLines []string) []PhalanxTunePos {
	loadTunerFile()
	var p Pos
	result := 0.0
	out := make([]PhalanxTunePos, 0, len(epdLines))

	for _, line := range epdLines {
		
		parseFEN(&p, line)

		result = 0.5
		if strings.Contains(line, "1-0") {
			result = 1.0
		} else if strings.Contains(line, "0-1") {
			result = 0.0
		}
		  

		baseScore := eval_internal(&p, false)
		if p.side == Black { baseScore = -baseScore }
		phase := extractPhase(line);

		var rec PhalanxTunePos
		rec.Result = result
		rec.BaseScore = baseScore
		rec.Phase = phase
		rec.Feature = extractFeatures(&p)

		out = append(out, rec)
	}

	return out
}

func extractPhase(fen string) int {
	phase := 0

	// Board part is the first space-separated field in FEN.
	board := fen
	if sp := strings.IndexByte(fen, ' '); sp >= 0 {
		board = fen[:sp]
	}

	for i := 0; i < len(board); i++ {
		switch board[i] {
		case 'n', 'N', 'b', 'B':
			phase += 1
		case 'r', 'R':
			phase += 2
		case 'q', 'Q':
			phase += 4
		}
	}

	if phase > 24 {
		phase = 24
	}
	return phase
}

// This needs to be changed, depending what feature we are tuning
func extractFeatures(p *Pos) [64]int {
	var feat [64]int

	whitePawns := p.pieceBB(White, P)
	blackPawns := p.pieceBB(Black, P)
	allWhitePawns := whitePawns
	allBlackPawns := blackPawns

	// White phalanx: pawn has a friendly pawn on adjacent file, same rank.
	for bb := whitePawns; bb != 0; bb &= bb - 1 {
		sq := lsb(bb)
		if isPhalanxPawn(allWhitePawns, sq) {
			feat[sq]++
		}
	}

	// Black phalanx: mirror square so one shared White-oriented table works.
	for bb := blackPawns; bb != 0; bb &= bb - 1 {
		sq := lsb(bb)
		if isPhalanxPawn(allBlackPawns, sq) {
			feat[sq^56]--
		}
	}

	return feat
}

func isPhalanxPawn(pawns uint64, sq int) bool{
     b := squareBit(sq)
     return shiftSides(b) & pawns != 0
}

func scorePhalanxTunePos(tp *PhalanxTunePos, phalanxMG, phalanxEG *[64]int) int {
	mg := 0
	eg := 0

	for sq := 0; sq < 64; sq++ {
		f := int(tp.Feature[sq])
		if f != 0 {
			mg += f * phalanxMG[sq]
			eg += f * phalanxEG[sq]
		}
	}

	phScore := (mg*tp.Phase + eg*(24-tp.Phase)) / 24
	return tp.BaseScore + phScore
}

func texelFitComposite(data []PhalanxTunePos) float64 {
	if len(data) == 0 {
		return 0.0
	}

	var total float64

	for i := range data {
		score := scorePhalanxTunePos(&data[i], &phalanxMG, &phalanxEG)
		pred := texelSigmoid(score, kConst)
		diff := data[i].Result - pred
		total += diff * diff
	}

	return total / float64(len(data))
}

func phalanxTuningSession() {
	loadTunerFile()
	if !tunerLoaded {
		fmt.Println("tuner data not loaded")
		return
	}

	fmt.Printf("Extracting phalanx tune data from %d positions...\n", len(epdLines))
	data := ExtractTuneData(epdLines)
	fmt.Printf("Extracted %d phalanx tuning records\n", len(data))

	startFit := texelFitComposite(data)
	fmt.Printf("Initial phalanx fit = %.6f\n", startFit)

	for i := 0; i < 100; i++ {
		fmt.Println("STAGE ", i)
	tune1DTableComposite(phalanxMG[:], "phalanxMG", true, data)
	tune1DTableComposite(phalanxEG[:], "phalanxEG", true, data)

	finalFit := texelFitComposite(data)
	fmt.Printf("Final phalanx fit = %.6f\n", finalFit)

	fmt.Println()
	printSinglePST("phalanxMG", "", &phalanxMG)
	fmt.Println()
	printSinglePST("phalanxEG", "", &phalanxEG)
	}
}

func tune1DTableComposite(table []int, tableName string, isPrinting bool, data []PhalanxTunePos) {
	bestFit := texelFitComposite(data)
	if isPrinting {
		fmt.Printf("Starting new batch for %s, initial fit = %.6f\n", tableName, bestFit)
	}

	for i := 0; i < len(table); i++ {
		orig := table[i]

		localBestVal, localBestFit, changed := tuneValueComposite(orig, bestFit, func(v int) {
			table[i] = v
		}, data)

		if changed {
			bestFit = localBestFit
			if isPrinting {
				fmt.Printf("%s[%d]: %d -> %d  fit = %.6f\n",
					tableName, i, orig, localBestVal, bestFit)
			}
		}
	}

	if isPrinting {
		fmt.Printf("Finished pass through %s, final fit = %.6f\n", tableName, bestFit)
	}
}

func tuneValueComposite(orig int, bestFit float64, set func(int), data []PhalanxTunePos) (int, float64, bool) {
	localBestVal := orig
	localBestFit := bestFit

	set(orig + 1)
	fitPlus := texelFitComposite(data)
	if fitPlus < localBestFit {
		localBestFit = fitPlus
		localBestVal = orig + 1
	}

	set(orig - 1)
	fitMinus := texelFitComposite(data)
	if fitMinus < localBestFit {
		localBestFit = fitMinus
		localBestVal = orig - 1
	}

	set(localBestVal)
	return localBestVal, localBestFit, localBestVal != orig
}