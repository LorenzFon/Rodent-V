// ================================================================
//
//	R O D E N T   V
//
// ================================================================
//
//	A Go chess engine by Naman Thanki and Pawel Koziol.
//	Based on Sungorus 1.4 by Pablo Vazquez (2013).
//
//	Authors        : Naman Thanki, Pawel Koziol
//	Date           : 2026
//
//	Every file is a short lesson in chess engine design. Follow the
//	table of contents below to understand the full pipeline from
//	board representation to UCI output.
//
//	Protocol: Universal Chess Interface (UCI)
//	Build:    go build -o rodent-v .

package main

import "fmt"

type SingleOption int

const (
	HcePerc SingleOption = iota
	NnuePerc
	NofSingleOptions
)

var SingleOptionName = [NofSingleOptions]string{
	HcePerc:  "hceWeight",
	NnuePerc: "nnueWeight",
}

var engineSide int
var singleOptions [NofSingleOptions]int
var optionPerColorValues [2][EvalComponentN]int

const weightOwn = 0
const weightOpp = 1

func init() {

	singleOptions[NnuePerc] = 80
	singleOptions[HcePerc] = 0

	for c := EvalComponent(0); c < EvalComponentN; c++ {
		optionPerColorValues[weightOwn][c] = 100
		optionPerColorValues[weightOpp][c] = 100
	}
}

func printUciOptionsPerColor() {
	for c := EvalComponent(0); c < EvalComponentN; c++ {
		fmt.Printf(
			"option name Own%s type spin default %d min 0 max 500\n",
			evalComponentName[c],
			optionPerColorValues[weightOwn][c],
		)
		fmt.Printf(
			"option name Opp%s type spin default %d min 0 max 500\n",
			evalComponentName[c],
			optionPerColorValues[weightOpp][c],
		)
	}
}
