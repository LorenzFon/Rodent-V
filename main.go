// ================================================================
//                       R O D E N T   V
// ================================================================
//
//   A Go chess engine by Naman Thanki and Pawel Koziol.
//   Based on Sungorus 1.4 by Pablo Vazquez (2013).
//
//   Authors        : Naman Thanki, Pawel Koziol
//   Date           : 2026
//
//   Every file is a short lesson in chess engine design. Follow the
//   table of contents below to understand the full pipeline from
//   board representation to UCI output.
//
//   Protocol: Universal Chess Interface (UCI)
//   Build:    go build -o rodent-v .
//
// ================================================================
//
//   TABLE OF CONTENTS  (one file per subsystem)
//   -------------------------------------------------------------
//   tables.go  - S1  constants, bit helpers, precomputed tables
//   pos.go     - S2  board representation and FEN parsing
//   attacks.go - S3  attack detection (is a square safe?)
//   moves.go   - S4  make / unmake move (incremental updates)
//   gen.go     - S5  move generation (captures and quiet moves)
//   legal.go   - S6  move legality validation
//   eval.go    - S7  static evaluation (material, mobility, pawns)
//   next.go    - S8  move ordering (TT -> good caps -> killers -> quiet)
//   trans.go   - S9  transposition table (4-bucket hash with aging)
//   swap.go    - S10 static exchange evaluation (SEE)
//   search.go  - S11 principal variation search + quiescence
//   uci.go     - S12 UCI protocol (commands, time management, perft)
//   main.go    -     entry point
//
// ================================================================

package main

import (
	"fmt"
	"os"
)

// init() is guaranteed to run before main()
func init() {
	engineSide = White
	initTables()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "genmagics" {
		FindMagics()
		return
	}

	// Tuner workflows are opt-in only and must be explicitly requested.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "tune":
			tv2Tune("quiet-labeled.epd", 5000, 1.0)
			return
		case "tunefile":
			if len(os.Args) < 3 {
				fmt.Println("usage: rodent-v tunefile <epd-or-book-file>")
				return
			}
			tv2Tune(os.Args[2], 5000, 1.0)
			return
			// case "loadsnapshot":
			// 	if len(os.Args) < 3 {
			// 		fmt.Println("usage: rodent-v loadsnapshot <snapshot-file>")
			// 		return
			// 	}
			// 	loadSnapshotFile(os.Args[2])
			// 	return
		}
	}

	uciLoop()
}

// pst debug functions, TODO: move them somewhere
func printPSToffsets(pst *[6][64]int) {
	fmt.Printf("offsets to move into material:\n")

	for piece := 0; piece < 6; piece++ {
		sum := 0
		for sq := 0; sq < 64; sq++ {
			sum += pst[piece][sq]
		}

		avg := float64(sum) / 64.0
		fmt.Printf("%s: sum = %d, avg = %.3f, material correction ~= %+d\n",
			pstLabels[piece], sum, avg, roundFloat(avg))
	}
}

func roundFloat(x float64) int {
	if x >= 0 {
		return int(x + 0.5)
	}
	return int(x - 0.5)
}
