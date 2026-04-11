package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
)

var featureMG = [64]int{
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
}

var featureEG = [64]int{
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
		   0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
           0,   0,   0,   0,   0,   0,   0,   0,
}

var texelK = 1.335

// All epds, regardless of score 
var epdLines[]string
var newTunerLoaded bool

func loadNewTunerFile() {
	if newTunerLoaded {
		return
	}

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

	// White feature
	for bb := whitePawns; bb != 0; bb &= bb - 1 {
		sq := lsb(bb)
		if hasFeature(allWhitePawns, sq) {
			feat[sq]++
		}
	}

	// Black feature (inverted)
	for bb := blackPawns; bb != 0; bb &= bb - 1 {
		sq := lsb(bb)
		if hasFeature(allBlackPawns, sq) {
			feat[sq^56]--
		}
	}

	return feat
}

func hasFeature(bitboard uint64, sq int) bool{
    b := squareBit(sq)
    return b & bitboard != 0
}

// texelNewSigmoid translates eval score into a winning probability
// between 0 and 1.
func texelNewSigmoid(score int, k float64) float64 {
	exponent := -(k * float64(score) / 400.0)
	return 1.0 / (1.0 + math.Pow(10.0, exponent))
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

type TunePos struct {
	Result    float64
	BaseScore int
	Phase     int
	Feature   [64]int
}

func ExtractTuneData(epdLines []string) []TunePos {

	var p Pos
	result := 0.0
	out := make([]TunePos, 0, len(epdLines))

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

		var rec TunePos
		rec.Result = result
		rec.BaseScore = baseScore
		rec.Phase = phase
		rec.Feature = extractFeatures(&p)

		out = append(out, rec)
	}

	return out
}

func scoreTunedElementPos(tp *TunePos, pawnMG, pawnEG *[64]int) int {
	mg := 0
	eg := 0

	for sq := 0; sq < 64; sq++ {
		f := int(tp.Feature[sq])
		if f != 0 {
			mg += f * pawnMG[sq]
			eg += f * pawnEG[sq]
		}
	}

	phScore := (mg*tp.Phase + eg*(24-tp.Phase)) / 24
	return tp.BaseScore + phScore
}

func texelFitComposite(data []TunePos) float64 {
	if len(data) == 0 {
		return 0.0
	}

	var total float64

	for i := range data {
		score := scoreTunedElementPos(&data[i], &featureMG, &featureEG)
		pred := texelNewSigmoid(score, texelK)
		diff := data[i].Result - pred
		total += diff * diff
	}

	return total / float64(len(data))
}

func tuneValueComposite(orig int, bestFit float64, set func(int), data []TunePos) (int, float64, bool) {
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

func tune1DTableComposite(table []int, tableName string, isPrinting bool, data []TunePos) {
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

func pawnTuningSession() {
	loadNewTunerFile()
	if !newTunerLoaded {
		fmt.Println("tuner data not loaded")
		return
	}

	fmt.Printf("Extracting pawn tune data from %d positions...\n", len(epdLines))
	data := ExtractTuneData(epdLines)
	fmt.Printf("Extracted %d pawn tuning records\n", len(data))

	startFit := texelFitComposite(data)
	fmt.Printf("Initial pawn fit = %.6f\n", startFit)

	for i := 0; i < 100; i++ {
		fmt.Println("STAGE ", i)
	tune1DTableComposite(featureMG[:], "pawnMG", true, data)
	tune1DTableComposite(featureEG[:], "pawnEG", true, data)

	finalFit := texelFitComposite(data)
	fmt.Printf("Final defended fit = %.6f\n", finalFit)

	fmt.Println()
	printSinglePST("pawnMG", "", &featureMG)
	fmt.Println()
	printSinglePST("pawnEG", "", &featureEG)
	}
}