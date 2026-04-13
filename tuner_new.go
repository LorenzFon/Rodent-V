package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
)

// ------------------------------------------------------------
// Extracted-feature gradient descent tuner for full [6][64] PST.
// Requires access to the following elements of the engine:
//
//   - type Pos
//   - parseFEN(*Pos, string)
//   - eval_internal(*Pos, bool) int
//   - constants White, Black, P, N, B, R, Q, K
//   - methods p.pieceBB(side, piece int) uint64
//   - lsb(uint64) int
//
// IMPORTANT:
//   1. ALL PST code must be DISABLED inside eval_internal(),
//      otherwise BaseScore will double-count PST. You can do it
//      by commenting out the body of addPst function.
//   2. This tunes White-oriented PSTs, mirroring Black squares by sq^56.
// ------------------------------------------------------------

var texelK = 1.335

// Float working PSTs.
var pstMGf [6][64]float64
var pstEGf [6][64]float64

type TuneData struct {
	Result    float64
	BaseScore int
	Phase     int
	Feature   [6][64]int
}

// All epds
var epdLines []string
var newTunerLoaded bool

// ------------------------------------------------------------
// Loader
// ------------------------------------------------------------

func loadNewTunerFile() {
	if newTunerLoaded {
		return
	}

	epdFile, err := os.Open("quiet-labeled.epd")
	fmt.Printf("reading epd file 'quiet-labeled.epd' (%v)\n", err == nil)
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

		epdLines = append(epdLines, line)
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("error while reading epd file: %v\n", err)
	}

	newTunerLoaded = true
	fmt.Printf("%d Total positions loaded\n", readCnt)
}

func newTunerFree() {
	epdLines = nil
	newTunerLoaded = false
}

// ------------------------------------------------------------
// Data extraction
// ------------------------------------------------------------

func extracTuneData(lines []string) []TuneData {
	var p Pos
	out := make([]TuneData, 0, len(lines))

	for _, line := range lines {
		parseFEN(&p, line)

		result := 0.5
		if strings.Contains(line, "1-0") {
			result = 1.0
		} else if strings.Contains(line, "0-1") {
			result = 0.0
		}

		baseScore := eval_internal(&p, false)
		if p.side == Black {
			baseScore = -baseScore
		}

		var rec TuneData
		rec.Result = result
		rec.BaseScore = baseScore
		rec.Phase = extractPhase(line)
		rec.Feature = extractFeatures(&p)

		out = append(out, rec)
	}

	return out
}

func extractPhase(fen string) int {
	phase := 0

	board := fen
	for i := 0; i < len(board); i++ {
		if board[i] == ' ' {
			board = board[:i]
			break
		}
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

func extractFeatures(p *Pos) [6][64]int {
	var feat [6][64]int

	for piece := P; piece <= K; piece++ {
		for bb := p.pieceBB(White, piece); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)
			feat[piece][sq]++
		}
		for bb := p.pieceBB(Black, piece); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)
			feat[piece][sq^56]--
		}
	}

	return feat
}

// ------------------------------------------------------------
// Scoring and loss
// ------------------------------------------------------------

func scoreExtractedFeatures(tp *TuneData, mgTable, egTable *[6][64]float64) float64 {
	mg := 0.0
	eg := 0.0

	for piece := P; piece <= K; piece++ {
		for sq := 0; sq < 64; sq++ {
			f := float64(tp.Feature[piece][sq])
			if f != 0.0 {
				mg += f * mgTable[piece][sq]
				eg += f * egTable[piece][sq]
			}
		}
	}

	tuneScore := (mg*float64(tp.Phase) + eg*float64(24-tp.Phase)) / 24.0
	return float64(tp.BaseScore) + tuneScore
}

func texelSigmoidFloat(score float64, k float64) float64 {
	exponent := -(k * score / 400.0)
	return 1.0 / (1.0 + math.Pow(10.0, exponent))
}

func texelSigmoidDerivative(score float64, k float64) float64 {
	p := texelSigmoidFloat(score, k)
	return math.Ln10 * k / 400.0 * p * (1.0 - p)
}

func texelFitOnData(data []TuneData) float64 {
	if len(data) == 0 {
		return 0.0
	}

	total := 0.0
	for i := range data {
		score := scoreExtractedFeatures(&data[i], &pstMGf, &pstEGf)
		prediction := texelSigmoidFloat(score, texelK)
		diff := data[i].Result - prediction
		total += diff * diff
	}
	return total / float64(len(data))
}

// ------------------------------------------------------------
// Gradient descent
// ------------------------------------------------------------

func gradientDescentEpoch(data []TuneData, lr float64) float64 {
	var gradMG [6][64]float64
	var gradEG [6][64]float64
	totalLoss := 0.0

	for i := range data {
		tunerData := &data[i]

		score := scoreExtractedFeatures(tunerData, &pstMGf, &pstEGf)
		prediction := texelSigmoidFloat(score, texelK)
		diff := prediction - tunerData.Result
		totalLoss += diff * diff

		dLossDScore := 2.0 * diff * texelSigmoidDerivative(score, texelK)

		mgFactor := float64(tunerData.Phase) / 24.0
		egFactor := float64(24-tunerData.Phase) / 24.0

		for piece := P; piece <= K; piece++ {
			for sq := 0; sq < 64; sq++ {
				f := float64(tunerData.Feature[piece][sq])
				if f != 0.0 {
					gradMG[piece][sq] += dLossDScore * f * mgFactor
					gradEG[piece][sq] += dLossDScore * f * egFactor
				}
			}
		}
	}

	if len(data) > 0 {
		scale := 1.0 / float64(len(data))
		for piece := P; piece <= K; piece++ {
			for sq := 0; sq < 64; sq++ {
				pstMGf[piece][sq] -= lr * gradMG[piece][sq] * scale
				pstEGf[piece][sq] -= lr * gradEG[piece][sq] * scale
			}
		}
		totalLoss *= scale
	}

	return totalLoss
}

func gradientDescentSession(data []TuneData, epochs int, lr float64) {
	startLoss := texelFitOnData(data)
	fmt.Printf("Initial loss = %.10f\n", startLoss)

	prevLoss := startLoss
	stall := 0

	for epoch := 0; epoch < epochs; epoch++ {
		loss := gradientDescentEpoch(data, lr)
		fmt.Printf("epoch %d  loss = %.10f\n", epoch, loss)
		if epoch%100 == 0 {
			printSnapshot()
		}

		if math.Abs(prevLoss-loss) < 1e-10 {
			stall++
			if stall >= 20 {
				fmt.Println("Stopping: loss no longer changes materially.")
				break
			}
		} else {
			stall = 0
		}
		prevLoss = loss
	}

	finalLoss := texelFitOnData(data)
	fmt.Printf("Final loss = %.10f\n", finalLoss)

	fmt.Println()
	printPSTSetAsInt("pstMG", &pstMGf)
	fmt.Println()
	printPSTSetAsInt("pstEG", &pstEGf)
}

// ------------------------------------------------------------
// Initialization helpers
// ------------------------------------------------------------

func initFromExistingTables(srcMG, srcEG *[6][64]int) {
	for piece := P; piece <= K; piece++ {
		for sq := 0; sq < 64; sq++ {
			pstMGf[piece][sq] = float64(srcMG[piece][sq])
			pstEGf[piece][sq] = float64(srcEG[piece][sq])
		}
	}
}

// ------------------------------------------------------------
// Printing helpers
// ------------------------------------------------------------

func roundToInt(x float64) int {
	if x >= 0 {
		return int(x + 0.5)
	}
	return int(x - 0.5)
}

func printSinglePSTFloatAsInt(label string, table *[64]float64) {
	fmt.Printf("\t%s: {\n", label)
	for row := 0; row < 8; row++ {
		start := row * 8
		fmt.Print("\t\t")
		for i := 0; i < 8; i++ {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%4d", roundToInt(table[start+i]))
		}
		fmt.Println(",")
	}
	fmt.Println("\t},")
}

// print tuned PST converted to int, so that it can be pasted
// directly into source code
func printPSTSetAsInt(varName string, pst *[6][64]float64) {
	fmt.Printf("var %s = [6][64]int{\n", varName)
	for piece := P; piece <= K; piece++ {
		printSinglePSTFloatAsInt(pstLabels[piece], &pst[piece])
	}
	fmt.Println("}")
}

// Optional: print deltas against initial PSTs.
func printPSTDeltaSet(varName string, base *[6][64]int, tuned *[6][64]float64) {
	fmt.Printf("var %s = [6][64]int{\n", varName)
	for piece := P; piece <= K; piece++ {
		fmt.Printf("\t%s: {\n", pstLabels[piece])
		for row := 0; row < 8; row++ {
			start := row * 8
			fmt.Print("\t\t")
			for i := 0; i < 8; i++ {
				if i > 0 {
					fmt.Print(", ")
				}
				d := roundToInt(tuned[piece][start+i]) - base[piece][start+i]
				fmt.Printf("%4d", d)
			}
			fmt.Println(",")
		}
		fmt.Println("\t},")
	}
	fmt.Println("}")
}

func printSnapshot() {
	fmt.Println()
	printPSTDeltaSet("pstDeltaMG", &pstMG, &pstMGf)
	fmt.Println()
	printPSTDeltaSet("pstDeltaEG", &pstEG, &pstEGf)
    fmt.Println()
    printPSTSetAsInt("pstMG", &pstMGf)
	fmt.Println()
	printPSTSetAsInt("pstEG", &pstEGf)
}

// ------------------------------------------------------------
// Entry point
// ------------------------------------------------------------

func gradientTuneSession(epochs int, lr float64) {
	loadNewTunerFile()
	if !newTunerLoaded {
		fmt.Println("tuner data not loaded")
		return
	}

	data := extracTuneData(epdLines)
	fmt.Printf("Extracted %d positions\n", len(data))

	initFromExistingTables(&pstMG, &pstEG)
	gradientDescentSession(data, epochs, lr)
	printSnapshot()

}