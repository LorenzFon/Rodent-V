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

var nnuePercentage int
var hcePercentage int
var engineSide int
var optionValues [2][EvalComponentN]int

const weightOwn = 0
const weightOpp = 1

func init() {

	nnuePercentage = 30
	hcePercentage = 60

	for c := EvalComponent(0); c < EvalComponentN; c++ {
		optionValues[weightOwn][c] = 100
		optionValues[weightOpp][c] = 100
	}
}

func printUciOptions() {
	for c := EvalComponent(0); c < EvalComponentN; c++ {
		fmt.Printf(
			"option name Own%s type spin default %d min 0 max 500\n",
			evalComponentName[c],
			optionValues[weightOwn][c],
		)
		fmt.Printf(
			"option name Opp%s type spin default %d min 0 max 500\n",
			evalComponentName[c],
			optionValues[weightOpp][c],
		)
	}
}
