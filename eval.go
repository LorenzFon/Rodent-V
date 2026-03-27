// ================================================================
// S7  STATIC EVALUATION
// ================================================================
//
//   evaluate(p) returns a score (in centipawns) from the perspective
//   of the side to move: positive = better for the mover, negative =
//   worse. The score is always in the range [-maxEval, +maxEval].
//
//   COMPONENTS (applied symmetrically for both sides, then differenced)
//   ------------------------------------------------------------------
//   1. MATERIAL BALANCE
//      The most important term.  Maintained incrementally in Pos so
//      this is just a subtraction.
//
//   2. MOBILITY
//      Only computed when material is nearly balanced (within 200 cp),
//      because in unbalanced positions a material term already dominates.
//      Bishops x4, Rooks x2, Queens x1. Heavier weights for
//      pieces whose mobility matters most strategically.
//
//   3. PIECE-SQUARE TABLES
//      Geometrical bonuses encouraging centralisation.  Also maintained
//      incrementally in Pos; again just a subtraction.
//
//   4. PAWN STRUCTURE
//      Passed pawns: bonus that grows with rank (closer to promotion).
//      Isolated pawns: penalty when no friendly pawn stands on an
//      adjacent file.
//
//   5. KING SAFETY
//      In the middlegame (opponent has a queen AND material > 1600 cp),
//      penalise a centralised king.  We negate the PST king bonus
//      (which rewards centralisation) to push the king toward the
//      corners where it is safer.
//

package main

// mobility counts a weighted sum of squares reachable by sliders:
//   bishop moves x4, rook moves x2, queen moves x1.
// A mobile piece is an active piece.
func mobility(p *Pos, side int) int {
	occ := p.occupied()
	mob := 0

	pieces := p.pieceBB(side, B)
	for pieces != 0 {
		from := lsb(pieces)
		mob += popCount(bishopAttacks(occ, from)) * 4
		pieces &= pieces - 1
	}

	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		from := lsb(pieces)
		mob += popCount(rookAttacks(occ, from)) * 2
		pieces &= pieces - 1
	}

	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		from := lsb(pieces)
		mob += popCount(queenAttacks(occ, from))
		pieces &= pieces - 1
	}

	return mob
}

// evaluatePawns scores the pawn structure for one side.
//
//   Passed pawn (+25..+50 cp): no enemy pawn can block or capture it
//   on the same or adjacent file ahead of it.  The bonus grows with
//   rank; a pawn on the 7th rank is almost a queen.
//
//   Isolated pawn (-20 cp): no friendly pawn on an adjacent file.
//   Isolated pawns cannot be defended by other pawns and are easy
//   targets for the opponent's rooks.
func evaluatePawns(p *Pos, side int) int {
	score  := 0
	pieces := p.pieceBB(side, P)

	for pieces != 0 {
		from := lsb(pieces)
		// Passed pawn: no enemy pawns in front on same or adjacent files.
		if passedMask[side][from]&p.pieceBB(opp(side), P) == 0 {
			score += passedBonus[side][rankOf(from)]
		}
		// Isolated pawn: no friendly pawns on adjacent files.
		if adjFileMask[fileOf(from)]&p.pieceBB(side, P) == 0 {
			score -= 20
		}
		pieces &= pieces - 1
	}
	return score
}

// evaluateKing penalises a centralised king in the middlegame.
// The king's PST bonus (which rewards centralisation) is negated
// when the opponent has a queen and significant material, that is,
// when it is dangerous for the king to stand in the open.
func evaluateKing(p *Pos, side int) int {
	if p.pieceBB(opp(side), Q) == 0 || p.material[opp(side)] <= 1600 {
		return 0 // endgame: king activity is beneficial
	}
	return -2 * pstTable[K][p.kingSq[side]]
}

// evaluate returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
func evaluate(p *Pos) int {
	score := p.material[White] - p.material[Black]

	// Mobility is only worth computing when the position is roughly level.
	if score > -200 && score < 200 {
		score += mobility(p, White) - mobility(p, Black)
	}

	score += p.pstScore[White] - p.pstScore[Black]
	score += evaluatePawns(p, White) - evaluatePawns(p, Black)
	score += evaluateKing(p, White) - evaluateKing(p, Black)

	// Clamp to the range that the transposition table can distinguish
	// from a forced mate score.
	if score < -maxEval {
		score = -maxEval
	} else if score > maxEval {
		score = maxEval
	}

	// Return score from the perspective of the side to move.
	if p.side == White {
		return score
	}
	return -score
}
