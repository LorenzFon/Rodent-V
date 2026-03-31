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

// init() is guaranteed to run before main()
func init() {
	initTables()
}

func main() {
	// stuff like reading data from file should go here
	// to handle errors
	uciLoop()
}
