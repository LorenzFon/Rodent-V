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
//      The most important term. Different values for midgame/endgame,
//      interpolated according to game phase.
//
//   2. MOBILITY
//      Bigger values for weaker pieces, major pieces gain in the endgame.
//
//   3. PIECE-SQUARE TABLES
//      Bonuses for occupying good squares, m*luses for occupying bad ones;
//      skeleton for eval functions. Different values for midgame/endgame,
//      interpolated according to game phase. Right now we use PeSTo tables,
//      modified to cater for existence of passed pawn eval.
//
//   4. PAWN STRUCTURE
//      Passed pawns: bonus that grows with rank (closer to promotion).
//      Isolated pawns: penalty when no friendly pawn stands on an
//      adjacent file.
//
//   5. KING SAFETY
//      To be ported from Rodent
//

package main

// --- Bitmasks used in eval only, put into init() to preserve locality ---
var (
	passedMask  [2][64]uint64
	adjFileMask [8]uint64
)

func init() {
	// --- Passed pawn masks ---
	// passedMask[White][sq]: squares strictly in front of sq on the
	// same and adjacent files.  A White pawn on sq is "passed" if
	// none of these squares contain a Black pawn.
	for sq := 0; sq < 64; sq++ {
		passedMask[White][sq] = 0
		for f := fileOf(sq) - 1; f <= fileOf(sq)+1; f++ {
			if f < 0 || f > 7 {
				continue
			}
			for r := rankOf(sq) + 1; r <= 7; r++ {
				passedMask[White][sq] |= squareBit(makeSquare(f, r))
			}
		}
	}
	for sq := 0; sq < 64; sq++ {
		passedMask[Black][sq] = 0
		for f := fileOf(sq) - 1; f <= fileOf(sq)+1; f++ {
			if f < 0 || f > 7 {
				continue
			}
			for r := rankOf(sq) - 1; r >= 0; r-- {
				passedMask[Black][sq] |= squareBit(makeSquare(f, r))
			}
		}
	}

	// --- Adjacent file masks ---
	// adjFileMask[f]: bitboard of the two files neighboring file f.
	// A pawn is isolated if adjFileMask[file] & ownPawns == 0.
	for f := 0; f < 8; f++ {
		adjFileMask[f] = 0
		if f > 0 {
			adjFileMask[f] |= fileABB << uint(f-1)
		}
		if f < 7 {
			adjFileMask[f] |= fileABB << uint(f+1)
		}
	}
}

// --- Eval params ---
var pieceValMG = [7]int{82, 337, 365, 477, 1025, 0, 0}
var pieceValEG = [7]int{94, 281, 297, 513, 937, 0, 0}

// bishopPairMG/EG: bonus for owning both bishops.
// The EG value is higher because open boards in the endgame
// let the bishop pair dominate knight+bishop or two knights.
const bishopPairMG = 20
const bishopPairEG = 60

// Rook on open/semi-open file bonuses.
// Open file (no pawns at all): bigger bonus since the rook has full
// penetration potential.  Semi-open (no own pawn, enemy pawn present):
// smaller bonus; the rook pressures the enemy pawn but is partly blocked.
// EG values are near-zero: open files drive MG tactics, not endgame play.
const rookOpenFileMG = 30
const rookOpenFileEG = 3
const rookSemiOpenFileMG = 20
const rookSemiOpenFileEG = -1

// minorHomeBB[side]: bitboard of the four squares where knights and bishops
// start the game.  A minor still on one of these squares counts as undeveloped.
// White: b1=1, c1=2, f1=5, g1=6   Black: b8=57, c8=58, f8=61, g8=62
var minorHomeBB = [2]uint64{
	White: (1 << 1) | (1 << 2) | (1 << 5) | (1 << 6),
	Black: (1 << 57) | (1 << 58) | (1 << 61) | (1 << 62),
}

// devPenaltyScale: multiplier for the quadratic undevelopment penalty.
// penalty = undeveloped^2 * devPenaltyScale  (MG only)
// 1 undeveloped:  -5 cp   (barely noticeable)
// 2 undeveloped: -20 cp   (a tempo behind)
// 3 undeveloped: -45 cp   (serious lag)
// 4 undeveloped: -80 cp   (opening disaster)
const devPenaltyScale = 5

// doubledPawnMG / doubledPawnEG: penalty for the rear pawn of a doubled
// pair, indexed by distance-to-edge (0=a/h file, 1=b/g, 2=c/f, 3=d/e).
// The penalty is applied only when the doubled pawn cannot immediately
// capture an enemy pawn (if it can capture, the structure is likely to be
// resolved tactically so the positional penalty is inappropriate).
// Values are mostly EG-heavy: doubled pawns become most dangerous as the
// position simplifies, since they cannot create a passed pawn by themselves.
var doubledPawnMG = [4]int{0, 3, -6, -14}
var doubledPawnEG = [4]int{-48, -38, -28, -10}

var pstMG = [6][64]int{
	P: {
		0, 0, 0, 0, 0, 0, 0, 0, -35, -6, -25, -22, -15, 18, 25, -26, /* pawn   r1-r2 */
		-26, -11, -4, -8, 5, 5, 22, -12, -29, -5, -4, 14, 17, 6, 8, -27, /*        r3-r4 */
		-16, 12, 8, 23, 25, 14, 18, -25, -6, 8, 19, 23, 38, 59, 26, -19, /*        r5-r6 */
		80, 76, 49, 54, 50, 57, 26, -3, 0, 0, 0, 0, 0, 0, 0, 0,
	},
	N: {
		-106, -19, -56, -31, -15, -26, -20, -22, -27, -51, -10, -1, 1, 20, -12, -17, /* knight r1-r2 */
		-25, -7, 10, 12, 21, 19, 27, -18, -12, 6, 16, 12, 30, 20, 23, -7, /*        r3-r4 */
		-7, 18, 18, 55, 35, 70, 17, 23, -47, 61, 37, 63, 85, 128, 74, 42, /*        r5-r6 */
		-71, -40, 74, 37, 23, 64, 5, -15, -165, -87, -32, -47, 63, -96, -15, -105,
	},
	B: {
		-32, -1, -12, -19, -11, -14, -39, -21, 5, 19, 17, 0, 9, 23, 36, 2, /* bishop r1-r2 */
		-2, 17, 15, 13, 12, 28, 19, 8, -4, 15, 11, 27, 33, 10, 9, 5, /*        r3-r4 */
		-3, 3, 19, 52, 35, 35, 5, -3, -18, 38, 43, 38, 35, 52, 37, -4, /*        r5-r6 */
		-26, 17, -17, -12, 32, 60, 20, -47, -27, 5, -82, -36, -23, -40, 8, -7,
	},
	R: {
		-17, -11, 2, 15, 14, 9, -39, -25, -43, -15, -18, -10, -1, 13, -4, -72, /* rook   r1-r2 */
		-44, -23, -16, -17, 1, 2, -3, -34, -38, -24, -13, -3, 9, -6, 6, -25, /*        r3-r4 */
		-22, -10, 5, 25, 22, 35, -8, -20, -6, 19, 24, 34, 15, 46, 61, 16, /*        r5-r6 */
		25, 30, 56, 60, 78, 65, 24, 42, 33, 42, 31, 49, 62, 11, 33, 45,
	},
	Q: {
		-2, -17, -7, 12, -13, -23, -29, -49, -34, -9, 11, 4, 10, 17, -1, 3, /* queen  r1-r2 */
		-16, 0, -13, -4, -7, 0, 13, 5, -11, -28, -11, -12, -4, -6, 1, -5, /*        r3-r4 */
		-29, -29, -18, -18, -3, 15, -3, -1, -11, -19, 5, 6, 29, 58, 47, 57, /*        r5-r6 */
		-23, -41, -5, 3, -17, 59, 29, 56, -26, 1, 31, 13, 61, 46, 45, 47,
	},
	K: {
		-17, 36, 14, -56, 6, -26, 26, 12, 1, 8, -6, -66, -45, -14, 11, 7, /* king   r1-r2 */
		-13, -12, -20, -48, -46, -28, -13, -25, -48, 1, -25, -41, -48, -42, -32, -53, /*        r3-r4 */
		-16, -18, -10, -29, -31, -25, -13, -35, -7, 26, 4, -17, -22, 8, 24, -24, /*        r5-r6 */
		30, 1, -18, -5, -10, -2, -36, -28, -66, 24, 18, -14, -58, -32, 3, 13,
	},
}

var pstEG = [6][64]int{
	P: {
		0, 0, 0, 0, 0, 0, 0, 0, 14, 6, 8, 8, 12, -2, 0, -9, /* pawn   r1-r2 */
		2, 5, -8, 0, 0, -5, -3, -10, 11, 7, -5, -9, -9, -10, 1, -3, /*        r3-r4 */
		30, 22, 11, 3, -4, 2, 15, 15, 72, 69, 46, 25, 24, 31, 52, 62, /*        r5-r6 */
		95, 92, 86, 62, 65, 88, 93, 124, 0, 0, 0, 0, 0, 0, 0, 0,
	},
	N: {
		-27, -49, -21, -13, -20, -16, -48, -62, -40, -18, -8, -3, 0, -18, -21, -42, /* knight r1-r2 */
		-21, -2, -1, 16, 12, -2, -18, -20, -16, -4, 18, 27, 17, 17, 6, -16, /*        r3-r4 */
		-15, 5, 24, 24, 24, 12, 10, -20, -22, -18, 9, 10, -3, -11, -19, -42, /*        r5-r6 */
		-23, -6, -24, 0, -9, -27, -26, -50, -56, -36, -11, -26, -30, -25, -61, -98,
	},
	B: {
		-21, -7, -21, -3, -7, -14, -3, -15, -12, -16, -5, 1, 5, -7, -13, -26, /* bishop r1-r2 */
		-10, -1, 10, 10, 15, 2, -5, -13, -4, 4, 14, 20, 7, 9, -2, -7, /*        r3-r4 */
		-1, 11, 13, 9, 14, 8, 4, 4, 4, -7, 0, 0, 0, 6, 2, 6, /*        r5-r6 */
		-6, -2, 9, -10, -2, -11, -4, -12, -12, -19, -9, -6, -5, -7, -15, -22,
	},
	R: {
		-7, 4, 5, -1, -3, -11, 6, -18, -4, -4, 2, 4, -7, -7, -9, -1, /* rook   r1-r2 */
		-2, 2, -3, 1, -5, -10, -6, -14, 5, 7, 10, 3, -4, -4, -6, -9, /*        r3-r4 */
		6, 5, 15, 0, 0, 3, 0, 4, 9, 9, 7, 4, 4, -1, -3, -1, /*        r5-r6 */
		9, 11, 11, 9, -5, 1, 6, 1, 15, 11, 20, 13, 12, 14, 10, 7,
	},
	Q: {
		-33, -27, -21, -41, -3, -31, -18, -40, -20, -21, -28, -15, -15, -21, -34, -31, /* queen  r1-r2 */
		-14, -26, 15, 7, 10, 18, 12, 7, -17, 30, 19, 48, 30, 36, 39, 25, /*        r3-r4 */
		4, 23, 23, 46, 59, 40, 59, 38, -19, 6, 9, 50, 49, 37, 19, 11, /*        r5-r6 */
		-16, 22, 33, 43, 60, 27, 32, 1, -7, 24, 23, 29, 29, 21, 12, 22,
	},
	K: {
		-55, -36, -19, -12, -30, -12, -26, -45, -28, -9, 6, 11, 12, 6, -3, -19, /* king   r1-r2 */
		-21, -1, 13, 19, 21, 18, 9, -10, -19, -3, 23, 22, 25, 25, 11, -12, /*        r3-r4 */
		-10, 24, 26, 25, 24, 35, 28, 1, 10, 18, 24, 13, 18, 46, 45, 11, /*        r5-r6 */
		-12, 19, 16, 16, 15, 40, 25, 12, -74, -34, -18, -20, -13, 17, 5, -19,
	},
}

// passedBonusMG / passedBonusEG: bonus for a passed pawn indexed by
// [blocked][relativeRank].  relativeRank is 0 at own back rank and 7
// at the promotion square, so it is the same for White and Black.
//
// The curve is exponential: ranks 0-2 contribute nothing (pawn too far
// away to matter immediately).  Rank 3+ grows quickly.
// "blocked" = a piece stands on the push square; the pawn can't advance
// so its value is reduced significantly.
var passedBonusMG = [2][8]int{
	0: {0, 0, 0, -5, 10, 25, 90, 0},  // free: push square empty
	1: {0, 0, 0, -8, -5, 12, 45, 0},  // blocked: push square occupied
}
var passedBonusEG = [2][8]int{
	0: {0, 0, 0, 10, 30, 100, 165, 0}, // free
	1: {0, 0, 0, -10, -8, 35, 60, 0},  // blocked
}

// ourPasserProximityMG/EG: bonus when our king is close to the passer's
// push square, indexed by Chebyshev distance (0 = same square, 7 = far corner).
// A king escorting its passer is a major endgame advantage.
var ourPasserProximityMG = [8]int{60, 80, 28, -8, 0, 4, 16, 4}
var ourPasserProximityEG = [8]int{108, 78, 68, 58, 28, 18, 8, 14}

// theirPasserProximityMG/EG: bonus indexed by Chebyshev distance between
// the enemy king and the passer's push square.  Convention matches Sirius:
// a positive value at large distance means the enemy king is far away (good
// for us); a negative value at distance 0 means the enemy king blocks (bad).
var theirPasserProximityMG = [8]int{-62, 0, 24, 20, 10, 12, 18, 16}
var theirPasserProximityEG = [8]int{16, 0, 0, 24, 58, 74, 78, 64}

// kingAttackerWeight[pieceType]: how dangerous is each piece type
// when it attacks squares near the enemy king.
// Indexed P=0..Q=4; pawns and kings are handled separately.
var kingAttackerWeight = [6]int{0, 2, 2, 3, 5, 0}

// evaluate returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
func evaluate(p *Pos) int {
	return eval_internal(p, false)
}

// eval_trace describes engine's evaluation
func eval_trace(p *Pos) int {
	return eval_internal(p, true)
}

// eval_internal returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
func eval_internal(p *Pos, shouldReport bool) int {
	var e EvalData // Golang-specific: it will be initialized as all zeroes

	// King rings must be set before evaluatePieces so that attack
	// tracking against the enemy king zone is available.
	e.kingRing[White] = kingAtk[p.kingSq[White]]
	e.kingRing[Black] = kingAtk[p.kingSq[Black]]

	evaluatePieces(p, &e, White)
	evaluatePieces(p, &e, Black)
	evaluatePawns(p, &e, White)
	evaluatePawns(p, &e, Black)
	evaluateKing(p, &e, White)
	evaluateKing(p, &e, Black)
	// Threats use the fully-built attack maps from all evaluators above.
	evaluateThreats(p, &e, White)
	evaluateThreats(p, &e, Black)

	// Interpolate between game phases
    mg := e.sumMg(White) - e.sumMg(Black)
    eg := e.sumEg(White) - e.sumEg(Black)
	if e.phase > 24 {
		e.phase = 24
	}

	score := (mg*e.phase + eg*(24-e.phase)) / 24

	// Clamp to the range that the transposition table can distinguish
	// from a forced mate score.
	if score < -maxEval {
		score = -maxEval
	} else if score > maxEval {
		score = maxEval
	}

	if shouldReport {
		e.PrintEvalDetails(p)
	}

	// Return score from the perspective of the side to move.
	if p.side == White {
		return score
	}
	return -score
}

// evaluatePieces evaluates pieces (except pawns and king),
// sets game phase, and accumulates king-safety attack data.
func evaluatePieces(p *Pos, e *EvalData, side int) {
	occ := p.occupied()
	enemy := opp(side)
	enemyRing := e.kingRing[enemy]

	pieces := p.pieceBB(side, N)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[N], pieceValEG[N])
		addPST(e, side, N, sq)
		atks := knightAtk[sq]
		e.addAttacks(side, N, atks)
		mob := popCount(atks&^p.colorBB[side]) - 4
		add(e, side, EvalMobility, 3*mob, 3*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[N]
			e.attackCnt[side] += popCount(ringAtks)
		}
		e.phase += 1
		pieces &= pieces - 1
	}

	// X-ray occupancies: remove friendly pieces that a slider can see
	// through, so battery partners (doubled rooks, rook+queen,
	// bishop+queen) are treated as a single coordinated attack.
	occForBishop := occ ^ (p.pieceBB(side, B) | p.pieceBB(side, Q))
	occForRook := occ ^ (p.pieceBB(side, R) | p.pieceBB(side, Q))
	occForQueen := occ ^ (p.pieceBB(side, B) | p.pieceBB(side, R))

	pieces = p.pieceBB(side, B)
	if popCount(pieces) >= 2 {
		add(e, side, EvalOther, bishopPairMG, bishopPairEG)
	}
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[B], pieceValEG[B])
		addPST(e, side, B, sq)
		atks := bishopAttacks(occForBishop, sq)
		e.addAttacks(side, B, atks)
		mob := popCount(atks) - 6
		add(e, side, EvalMobility, 5*mob, 4*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[B]
			e.attackCnt[side] += popCount(ringAtks)
		}
		e.phase += 1
		pieces &= pieces - 1
	}

	// Quadratic undevelopment penalty: each minor still on its home square
	// compounds the punishment.  Two pieces at home is 4× worse than one,
	// four at home is 16× worse — reflecting how a crowded back rank
	// prevents castling and limits all piece coordination.
	minors := p.pieceBB(side, N) | p.pieceBB(side, B)
	undeveloped := popCount(minors & minorHomeBB[side])
	if undeveloped > 0 {
		add(e, side, EvalOther, -(undeveloped*undeveloped*devPenaltyScale), 0)
	}

	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[R], pieceValEG[R])
		addPST(e, side, R, sq)
		atks := rookAttacks(occForRook, sq)
		e.addAttacks(side, R, atks)
		mob := popCount(atks) - 7
		add(e, side, EvalMobility, 3*mob, 2*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[R]
			e.attackCnt[side] += popCount(ringAtks)
		}
		// Open / semi-open file bonus.
		fileMask := fileABB << uint(fileOf(sq))
		ownPawnsOnFile := fileMask & p.pieceBB(side, P)
		if ownPawnsOnFile == 0 {
			if fileMask&p.pieceBB(opp(side), P) == 0 {
				add(e, side, EvalOther, rookOpenFileMG, rookOpenFileEG)
			} else {
				add(e, side, EvalOther, rookSemiOpenFileMG, rookSemiOpenFileEG)
			}
		}
		e.phase += 2
		pieces &= pieces - 1
	}

	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[Q], pieceValEG[Q])
		addPST(e, side, Q, sq)
		atks := queenAttacks(occForQueen, sq)
		e.addAttacks(side, Q, atks)
		mob := popCount(atks) - 14
		add(e, side, EvalMobility, 2*mob, 2*mob)
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[Q]
			e.attackCnt[side] += popCount(ringAtks)
		}
		e.phase += 4
		pieces &= pieces - 1
	}
}

// evaluatePawns scores the pawn structure for one side.
//
//	Passed pawn (+25..+50 cp): no enemy pawn can block or capture it
//	on the same or adjacent file ahead of it.  The bonus grows with
//	rank; a pawn on the 7th rank is almost a queen.
//
//	Isolated pawn (-20 cp): no friendly pawn on an adjacent file.
//	Isolated pawns cannot be defended by other pawns and are easy
//	targets for the opponent's rooks.
func evaluatePawns(p *Pos, e *EvalData, side int) {

	pieces := p.pieceBB(side, P)

	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[P], pieceValEG[P])
		addPST(e, side, P, sq)
		e.addAttacks(side, P, pawnAtk[side][sq])

		// pushSq: the square directly in front of this pawn.
		// Pawns can't legally sit on the promotion rank, but guard anyway.
		pushSq := sq + 8
		if side == Black {
			pushSq = sq - 8
		}

		// Passed pawn: no enemy pawns in front on same or adjacent files.
		if passedMask[side][sq]&p.pieceBB(opp(side), P) == 0 {
			// Relative rank: 0 = own back rank, 7 = promotion square.
			var relRank int
			if side == White {
				relRank = rankOf(sq)
			} else {
				relRank = 7 - rankOf(sq)
			}

			// Blocked: any piece standing on the push square.
			// pushSq is valid for all legal pawn squares (rank 1..6 for White,
			// rank 6..1 for Black), but guard against the promotion edge just in case.
			blocked := 0
			if pushSq >= 0 && pushSq < 64 && p.board[pushSq] != NO_PC {
				blocked = 1
			}
			add(e, side, EvalPassers, passedBonusMG[blocked][relRank], passedBonusEG[blocked][relRank])

			// King proximity: meaningful only from rank 3+.
			// Our king wants to escort; enemy king wants to block.
			if relRank >= 3 && pushSq >= 0 && pushSq < 64 {
				ourDist := chebyshev(p.kingSq[side], pushSq)
				theirDist := chebyshev(p.kingSq[opp(side)], pushSq)
				add(e, side, EvalPassers, ourPasserProximityMG[ourDist], ourPasserProximityEG[ourDist])
				add(e, side, EvalPassers, theirPasserProximityMG[theirDist], theirPasserProximityEG[theirDist])

				// Slider behind: enemy rook or queen behind the passer on
				// the same file controls the promotion path.
				fileMask := fileABB << uint(fileOf(sq))
				var behindMask uint64
				if side == White {
					// squares below sq on the same file
					behindMask = fileMask & (squareBit(sq) - 1)
				} else {
					// squares above sq on the same file
					// squareBit(sq+1)-1 masks bits 0..sq, so complement gives sq+1..63
					behindMask = fileMask &^ (squareBit(sq+1) - 1)
				}
				enemySliders := p.pieceBB(opp(side), R) | p.pieceBB(opp(side), Q)
				if behindMask&enemySliders != 0 {
					add(e, side, EvalPassers, -25, -45)
				}
			}
		}
		// Isolated pawn: no friendly pawns on adjacent files.
		if adjFileMask[fileOf(sq)]&p.pieceBB(side, P) == 0 {
			add(e, side, EvalPawns, -20, -20)
		}
		// Doubled pawn: a friendly pawn stands directly ahead on the same file.
		// We only penalise when the pawn cannot immediately capture an enemy
		// pawn — if it can, the doubled structure is likely resolved tactically.
		// The penalty is indexed by distance-to-edge so central files (where
		// the doubled pawn blocks the most pawn breaks) are hurt the most in MG,
		// while edge files are punished more in EG (they can rarely promote).
		if side == White {
			pushSq = sq + 8
		} else {
			pushSq = sq - 8
		}
		if pushSq >= 0 && pushSq < 64 && p.pieceBB(side, P)&squareBit(pushSq) != 0 {
			// Only penalise if the doubled pawn has no immediate captures.
			if pawnAtk[side][sq]&p.pieceBB(opp(side), P) == 0 {
				fileIdx := fileOf(sq)
				if fileIdx > 3 {
					fileIdx = 7 - fileIdx
				}
				add(e, side, EvalPawns, doubledPawnMG[fileIdx], doubledPawnEG[fileIdx])
			}
		}
		pieces &= pieces - 1
	}
}

// pawnShieldMG computes the middlegame pawn-shield penalty for a king.
// We inspect the two ranks directly in front of the king on its file
// and the two adjacent files.  Missing pawns and open/semi-open files
// near the king are penalised.
func pawnShieldMG(p *Pos, side int) int {
	kSq := p.kingSq[side]
	kFile := fileOf(kSq)
	kRank := rankOf(kSq)

	ownPawns := p.pieceBB(side, P)
	enemyPawns := p.pieceBB(opp(side), P)

	penalty := 0

	for df := -1; df <= 1; df++ {
		f := kFile + df
		if f < 0 || f > 7 {
			continue
		}
		fileMask := fileABB << uint(f)

		// Ranks immediately in front of the king (r1 closer, r2 further).
		var r1, r2 int
		if side == White {
			r1, r2 = kRank+1, kRank+2
		} else {
			r1, r2 = kRank-1, kRank-2
		}

		hasPawnR1 := r1 >= 0 && r1 <= 7 && ownPawns&squareBit(makeSquare(f, r1)) != 0
		hasPawnR2 := r2 >= 0 && r2 <= 7 && ownPawns&squareBit(makeSquare(f, r2)) != 0

		if !hasPawnR1 && !hasPawnR2 {
			penalty += 28 // no shield pawn at all
		} else if !hasPawnR1 {
			penalty += 10 // pawn advanced one step
		}

		// Additional penalty for open / semi-open files through the king zone.
		if fileMask&ownPawns == 0 {
			if fileMask&enemyPawns == 0 {
				penalty += 15 // open file
			} else {
				penalty += 7 // semi-open file
			}
		}
	}

	return -penalty
}

// evaluateKing scores the king for one side: PST + pawn shield (MG) +
// king-attack danger based on what the *enemy* accumulated in
// evaluatePieces.
func evaluateKing(p *Pos, e *EvalData, side int) {
	sq := p.kingSq[side]
	addPST(e, side, K, sq)
	e.addAttacks(side, K, kingAtk[sq])

	// Pawn shield only matters in the middlegame.
	shieldMG := pawnShieldMG(p, side)
	add(e, side, EvalSafety, shieldMG, 0)

	// King-attack danger: pressure accumulated by the *enemy* on our
	// king ring.  We only trigger this when at least two distinct pieces
	// are bearing down on the king zone; a lone attacker is rarely fatal.
	enemy := opp(side)
	if e.attackCnt[enemy] >= 2 {
		// Scale danger by weight and count; kept intentionally modest so
		// the engine does not become reckless about piece sacrifices.
		danger := e.attackWt[enemy] * (e.attackCnt[enemy] + 2) / 8

		// Virtual checks: enemy pieces that can reach a checking square
		// right now (ignoring whether that square is defended).
		occ := p.occupied()
		knightChecks := popCount(p.pieceBB(enemy, N) & knightAtk[sq])
		bishopChecks := popCount(p.pieceBB(enemy, B) & bishopAttacks(occ, sq))
		rookChecks := popCount(p.pieceBB(enemy, R) & rookAttacks(occ, sq))
		queenChecks := popCount(p.pieceBB(enemy, Q) & queenAttacks(occ, sq))

		danger += knightChecks*3 + bishopChecks*3 + rookChecks*4 + queenChecks*6

		// Apply mostly to MG; small residual in EG so the eval is not
		// completely blind to king safety once queens are traded off.
		add(e, side, EvalSafety, -danger, -danger/4)
	}
}

// ---- Threat evaluation ----
//
// Threat scores reward the side whose pieces attack undefended or
// poorly-defended enemy pieces.  The bonus depends on:
//   - what piece type is doing the attacking
//   - what piece type is being attacked (victim)
//   - whether the victim is defended (index 0=hanging, 1=defended)
//
// Pawn and king threats do not use the defended flag: a pawn threat is
// always serious because capturing is free; a king threat is only
// rewarded when the victim is undefended (handled in the code).
//
// Push threats: a pawn one step away from attacking an enemy non-pawn.
// Only counted when the push square is safe (not controlled by an enemy pawn).

// threatByPawnMG/EG[victimType] — P..Q (K is never threatened by a pawn).
var threatByPawnMG = [6]int{-7, 73, 65, 72, 56, 0}
var threatByPawnEG = [6]int{-19, 41, 72, 50, 24, 0}

// threatByKnightMG/EG[defended][victimType] — 0=hanging, 1=defended.
var threatByKnightMG = [2][6]int{
	{5, 12, 50, 86, 41, 0},
	{-8, 9, 38, 71, 50, 0},
}
var threatByKnightEG = [2][6]int{
	{37, 85, 33, 13, 8, 0},
	{11, 79, 29, 45, 46, 0},
}

// threatByBishopMG/EG[defended][victimType].
var threatByBishopMG = [2][6]int{
	{3, 36, 12, 58, 61, 0},
	{-5, 20, 4, 56, 63, 0},
}
var threatByBishopEG = [2][6]int{
	{34, 44, 102, 35, 53, 0},
	{4, 21, 76, 60, 74, 0},
}

// threatByRookMG/EG[defended][victimType].
var threatByRookMG = [2][6]int{
	{-3, 35, 45, -12, 67, 0},
	{-10, 8, 19, 1, 54, 0},
}
var threatByRookEG = [2][6]int{
	{50, 52, 49, 50, -10, 0},
	{10, 15, 4, 22, 85, 0},
}

// threatByQueenMG/EG[defended][victimType].
var threatByQueenMG = [2][6]int{
	{8, 25, 18, 16, -2, 0},
	{-5, 2, -9, -7, -19, 0},
}
var threatByQueenEG = [2][6]int{
	{21, 30, 65, 12, -17, 0},
	{16, 8, 37, 7, 1, 0},
}

// threatByKingMG/EG[victimType] — king only attacks undefended squares.
var threatByKingMG = [6]int{39, 33, 99, 83, 0, 0}
var threatByKingEG = [6]int{18, 38, 33, 8, 0, 0}

// pushThreatMG/EG: per non-pawn enemy piece attacked by a safe pawn push.
const pushThreatMG = 13
const pushThreatEG = 17

// evaluateThreats scores the positional pressure exerted by (side)'s pieces
// on the enemy.  Must be called after all attack maps are fully built.
func evaluateThreats(p *Pos, e *EvalData, side int) {
	enemy := opp(side)
	enemyPieces := p.colorBB[enemy]

	// defendedBB: squares the enemy double-covers, or covers with a pawn,
	// or covers without us also double-covering.  Hitting a piece on a
	// defended square is less valuable than hitting a hanging piece.
	defendedBB := e.attackedBy2[enemy] |
		e.attackedBy[enemy][P] |
		(e.attacked[enemy] &^ e.attackedBy2[side])

	// Pawn threats: any enemy piece attacked by our pawns.
	pawnThreats := e.attackedBy[side][P] & enemyPieces
	for bb := pawnThreats; bb != 0; {
		sq := lsb(bb)
		bb &= bb - 1
		victim := p.typeAt(sq)
		add(e, side, EvalThreats, threatByPawnMG[victim], threatByPawnEG[victim])
	}

	// Minor/major piece threats with defended flag.
	for _, attacker := range []int{N, B, R, Q} {
		var mgTable, egTable *[2][6]int
		switch attacker {
		case N:
			mgTable, egTable = &threatByKnightMG, &threatByKnightEG
		case B:
			mgTable, egTable = &threatByBishopMG, &threatByBishopEG
		case R:
			mgTable, egTable = &threatByRookMG, &threatByRookEG
		case Q:
			mgTable, egTable = &threatByQueenMG, &threatByQueenEG
		}
		threats := e.attackedBy[side][attacker] & enemyPieces
		if attacker == Q {
			threats &^= p.pieceBB(enemy, K) // queen doesn't threaten king
		}
		for bb := threats; bb != 0; {
			sq := lsb(bb)
			bb &= bb - 1
			victim := p.typeAt(sq)
			defended := 0
			if defendedBB&squareBit(sq) != 0 {
				defended = 1
			}
			add(e, side, EvalThreats, mgTable[defended][victim], egTable[defended][victim])
		}
	}

	// King threats: king attacks undefended enemy pieces.
	kingThreats := e.attackedBy[side][K] & enemyPieces &^ defendedBB
	for bb := kingThreats; bb != 0; {
		sq := lsb(bb)
		bb &= bb - 1
		victim := p.typeAt(sq)
		add(e, side, EvalThreats, threatByKingMG[victim], threatByKingEG[victim])
	}

	// Push threats: safe pawn advances that would attack an enemy non-pawn.
	// Safe = the push square is not controlled by an enemy pawn.
	occ := p.occupied()
	ownPawns := p.pieceBB(side, P)
	nonPawnEnemies := enemyPieces &^ p.pieceBB(enemy, P)
	enemyPawnAtks := e.attackedBy[enemy][P]
	var pushes uint64
	if side == White {
		pushes = (ownPawns << 8) &^ occ
		// Double push from rank 2.
		pushes |= ((pushes & rank3BB) << 8) &^ occ
	} else {
		pushes = (ownPawns >> 8) &^ occ
		// Double push from rank 7 (relative).
		pushes |= ((pushes & rank6BB) >> 8) &^ occ
	}
	// Only safe pushes: not controlled by an enemy pawn.
	safePushes := pushes &^ enemyPawnAtks
	// Count safe pushes that would attack a non-pawn enemy.
	var pushThreatBB uint64
	if side == White {
		pushThreatBB = ((safePushes << 7) &^ fileHBB) | ((safePushes << 9) &^ fileABB)
	} else {
		pushThreatBB = ((safePushes >> 7) &^ fileABB) | ((safePushes >> 9) &^ fileHBB)
	}
	cnt := popCount(pushThreatBB & nonPawnEnemies)
	add(e, side, EvalThreats, cnt*pushThreatMG, cnt*pushThreatEG)
}

// TODO: initialize pst tables in a separate file and
// do not handle White and Black separately in addPST

// addPST adds the piece-square table score for a piece on sq.
func addPST(e *EvalData, side, piece, sq int) {
	if side == White {
		add(e, side, EvalPst, pstMG[piece][sq], pstEG[piece][sq])
		return
	}

	msq := sq ^ 56
	add(e, side, EvalPst, pstMG[piece][msq], pstEG[piece][msq])
}

// add adds MG/EG scores for one side to EvalData.
func add(e *EvalData, side int, component EvalComponent, mg, eg int) {
	e.mgScore[side] [component] += mg
	e.egScore[side] [component] += eg
}
