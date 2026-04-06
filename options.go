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

var optionValues[EvalComponentN] int

func init() {
	for c := EvalComponent(0); c < EvalComponentN; c++ {
		optionValues[c] = 100
	}
}

func printUciOptions() {
	for c := EvalComponent(0); c < EvalComponentN; c++ {
		fmt.Printf(
			"option name %s type spin default %d min 0 max 500\n",
			evalComponentName[c],
			optionValues[c],
		)
	}
}