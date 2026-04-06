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

	initEvalHash(128 * 128)
}

// --- Eval params ---
var pieceValMG = [7]int{82,  343, 365, 485, 1029, 0, 0}
var pieceValEG = [7]int{100, 273, 293, 523, 952, 0, 0}

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

// Piece/square tables are roughly centered around zero, which means that
// the sum of their values is close to zero. It has a few advantages:
// changing pst percentage value should not disturb engine's perception
// of material advantage, and changing pst to another zero-centered set
// should not require adjustement of material values.
var pstMG = [6][64]int{
	P: {
		   0,   0,   0,   0,   0,   0,   0,   0,
		 -37, -23, -24, -32, -30,   7,  12, -25,
		 -23, -24,  -1, -15,   0,   2,  16, -14,
		 -34, -25,  -1,  17,  22,  18,  -7, -25,
		 -20,  -1,   5,  29,  28,  20,   8, -23,
		 -12,  -3,  20,  22,  56,  67,  20, -21,
		  45,  36,   5,   9,  16,   9, -34, -71,
		   0,   0,   0,   0,   0,   0,   0,   0,
	},
	N: {
        -102,   2, -45, -34,   6, -17,   6, -30,
		 -21, -68,  -7,   9,  12,  23,  -8, -10,
		 -21,  -2,  12,  13,  36,  21,  25, -21,
		  -8,  -3,  10,  15,  33,  16,  16,   3,
		 -15,  16,   4,  51,  27,  74,  15,  19,
		 -48,  51,  26,  56,  84, 122,  81,  38,
		 -79, -40,  64,  24,  23,  63,   2,   0,
		-178, -87, -37, -41,  62, -103, -13, -102,
	},
	// NOTE: blobs of high values on ranks 4-6
	// and on central files indicates that we would
	// gain from bishop and knight outposts evaluation
	B: {
		 -37,   2,  11,  -7,  -9,  11, -41, -18,
		  16,  14,  14,  -2,   6,  12,  34,   7,
		   4,  14,   5,  -1,   4,  19,  17,   8,
		 -13,  14,   1,  24,  20,  -8,   6,   5,
		 -14,   4,   4,  33,  18,  24,   1,   0,
		 -19,  36,  44,  26,  24,  32,  17, -14,
		 -36,   5, -36, -25,  31,  53,  13, -47,
		 -25,   9, -89, -44, -34, -56,  11,  -1,
	},
	R: {
		 -12, -17,  -8,  -4,  -1,   1, -36, -13,
		 -37, -23, -34, -18,  -6,   5, -18, -68,
		 -48, -28, -30, -40, -10,  -7,   2, -37,
		 -39, -44, -38, -12,   0, -24,  -5, -29,
		 -31, -29,  -7,  14,   5,  31, -18, -30,
		 -12,  14,  10,  24,  13,  39,  56,  -4,
		  18,  27,  40,  47,  76,  65,  29,  35,
		  11,  35,  25,  47,  58,   7,  20,  33,
	},
	Q: {
		   4, -15, -10,  19, -14, -31, -45, -43,
		 -30,  -4,  14,   1,  14,  14,  -3,  14,
		 -12,   6, -12,  -5, -12, -10,  13,   7,
		   0, -40, -18, -26,  -7, -11,   5,  -1,
		 -32, -34, -21, -22, -19,   1,   1,  -3,
		 -14, -21,   8,  -7,  28,  62,  42,  53,
		 -23, -57, -11,  -4, -23,  50,  30,  60,
		 -20,   6,  25,   4,  62,  35,  40,  52,
	},
	K: {
		 -32,  28,  24, -43,  19, -29,  24,  -4,
		  -3,   4,  -4, -61, -42, -24,   9,   6,
		 -20,  -4, -25, -46, -47, -33,  -5, -36,
		 -56,  14, -18, -45, -55, -36, -29, -57,
		 -19, -28,  -1, -33, -29, -34, -16, -47,
		   5,  43,  19,  -7, -17,  24,  26, -30,
		  30,  14, -10,  23,   9,   3, -20, -41,
		 -47,  37,  20,  -5, -44, -29,  16,  28,
	},
}

var pstEG = [6][64]int{
	P: {
		   0,   0,   0,   0,   0,   0,   0,   0,
		   3, -20,  -7, -11,  -2, -17, -35, -21,
		 -13, -19, -25, -16, -19, -23, -40, -21,
		   2, -17, -20, -32, -29, -32, -29, -13,
		  23,  -3,  -4, -23, -26, -16, -11,   4,
		  86,  83,  65,  37,  22,  21,  50,  66,
		  47,  27,  20,  -9,  -6,  -2,  17,  59,
		   0,   0,   0,   0,   0,   0,   0,   0,
	},
	N: {
		 -13, -33,   6,   7,  -1,  10, -36, -42,
		 -17,   8,   8,   7,  12,   2,   8, -24,
		   6,  12,  10,  27,  13,   6,  -2,  11,
		   3,   9,  30,  39,  23,  26,  15,  -2,
		   5,  23,  35,  23,  35,  10,  25,   1,
		 -17,  -4,  18,  13,   5, -10, -14, -35,
		  -4,   8, -14,  18,   2, -14, -10, -45,
		 -22, -18,  14, -15,  -8,  -6, -58, -68,
	},
	B: {
		  -5,   7, -19,   5,   4,  -8,  10,  -1,
		 -10, -19,  -9,   3,   7,   5, -11, -13,
		  -4,  -3,  10,  11,  12,  -1,  -3,  -2,
		   5,   1,   8,   9,  -2,   9,  -2,   9,
		  10,   9,  11,  -5,   0,  -2,   2,   9,
		  12,  -5,  -6, -11,  -9,  -5,   6,  10,
		   9,  -7,  15, -10,  -1, -14,  -1,  -9,
		   3, -15,   4,  -8,   5,  10,  -6,  -4,
	},
	R: {
		  -8,   1,   8,  -1,  -5,  -3,   8, -22,
		   1,   2,   4,  -1,  -9, -14, -11,  10,
		   6,   4,  -6,   6, -12, -11, -15,  -6,
		   9,  14,  12,  -1,  -9,  -1,  -7, -10,
		   6,  10,  11,  -4,   0,  -6,   3,   3,
		  14,   1,   4,  -3,  -3, -18, -11,  -2,
		  15,  10,   3,   1, -14,  -8,   5,   5,
		  11,   1,  11,   4,   6,  12,   7,   6,
	},
	Q: {
		 -57, -41, -29, -65,  -2, -38, -17, -61,
		 -26, -30, -38, -20, -28, -33, -41, -35,
		 -26, -52,   4, -11,   6,  12,   1,   6,
		 -29,  37,  16,  37,  24,  24,  22,  23,
		   2,  22,  10,  32,  60,  31,  48,  32,
		 -34,  -3,  -3,  34,  29,  17,   3, -15,
		 -26,  17,  27,  37,  55,  18,  13, -17,
		 -13,  18,  22,  18,  15,  29,   1,  15,
	},
	K: {
		 -70, -51, -30,  -6, -33,  -8, -38, -63,
		 -37, -15,   9,  23,  20,  11, -10, -29,
		 -28,  -3,  15,  28,  31,  25,   6, -14,
		 -24,  -9,  28,  37,  39,  28,  13, -16,
		  -9,  29,  33,  42,  36,  45,  33,   8,
		   3,  19,  28,  22,  26,  43,  44,  10,
		 -14,  18,  19,  13,  21,  49,  39,  22,
		 -65, -39,  -9, -10, -10,  23,   8, -19,
	},
}

// passedBonusMG / passedBonusEG: bonus for a passed pawn indexed by
// [blocked][relativeRank].  relativeRank is 0 at own back rank and 7
// at the promotion square, so it is the same for White and Black.
//
// The values are tuned automatically, by a variant of Texel tuning
// that uses many small batches and I am deeply sorry how it turned out.
var passedBonusMG = [2][8]int{
        0: {0, 25, 20, -6, 5, 15, 83, 0}, // free: push square empty
        1: {0, 17, 23, 1, 17, 15, 87, 0}, // blocked: push square occupied
}
var passedBonusEG = [2][8]int{
        0: {0, 19, 23, 7, 35, 61, 184, 0}, // free
        1: {0, 20, 29, -6, 18, -11, 78, 0}, // blocked
}

// ourPasserProximityMG/EG: bonus when our king is close to the passer's
// push square, indexed by Chebyshev distance (0 = same square, 7 = far corner).
// A king escorting its passer is a major endgame advantage.
var ourPasserProximityMG = [8]int{61, 76, 32, -25, 0, -11, 26, 13}
var ourPasserProximityEG = [8]int{60, 55, 34, 30, 3, 2, -11, -10}

// theirPasserProximityMG/EG: bonus indexed by Chebyshev distance between
// the enemy king and the passer's push square.  Convention matches Sirius:
// a positive value at large distance means the enemy king is far away (good
// for us); a negative value at distance 0 means the enemy king blocks (bad).
var theirPasserProximityMG = [8]int{-26, 27, 37, 10, -4, -2, 13, 17}
var theirPasserProximityEG = [8]int{-32, -37, -16, 22, 52, 55, 50, 46}

// kingAttackerWeight[pieceType]: how dangerous is each piece type
// when it attacks squares near the enemy king.
// Indexed P=0..Q=4; pawns and kings are handled separately.
var kingAttackerWeight = [6]int{0, 2, 2, 3, 5, 0}

// evaluate returns the static score for the current position from the
// perspective of the side to move.  Positive = better for the mover.
func evaluate(p *Pos) int {
	if score, ok := probeEvalHash(p.key); ok {
		return score
	}

	score := eval_internal(p, false)
	storeEvalHash(p.key, score)
	return score
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
		e.addAttacks(side, B, bishopAttacks(occ, sq))
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
		add(e, side, EvalOther, -(undeveloped * undeveloped * devPenaltyScale), 0)
	}

	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[R], pieceValEG[R])
		addPST(e, side, R, sq)
		atks := rookAttacks(occForRook, sq)
		e.addAttacks(side, R, rookAttacks(occ, sq))
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
		e.addAttacks(side, Q, queenAttacks(occ, sq))
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
		pushThreatBB = ((safePushes >> 7) &^ fileHBB) | ((safePushes >> 9) &^ fileABB)
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
	e.mgScore[side][component] += mg
	e.egScore[side][component] += eg
}
