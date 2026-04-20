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
	supportMask [2][64]uint64
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

	// --- Support mask to detect backward pawns ---
	for sq := A1; sq < 64; sq++ {
		base := shiftSides(squareBit(sq))

		supportMask[White][sq] = base | fillSouth(base)
		supportMask[Black][sq] = base | fillNorth(base)
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
	initPawnHash(64 * 128)
}

// tunerDisableKingSafety: when true, evaluateKing skips the danger
// calculation and pawnShieldMG returns 0, so the tuner's BaseScore
// excludes king safety (the tuner adds it back via its own parameters).
// var tunerDisableKingSafety bool

// --- Eval params ---
var pieceValMG = [7]int{83, 343, 365, 485, 1029, 0, 0}
var pieceValEG = [7]int{100, 273, 293, 523, 952, 0, 0}

// mobility
var mobOffset = [7]int{0, 4, 6, 7, 14, 0, 0}
var mobMG = [7]int{0, 3, 5, 3, 2, 0, 0}
var mobEG = [7]int{0, 3, 4, 2, 2, 0, 0}

// bishopPairMG/EG: bonus for owning both bishops.
// The EG value is higher because open boards in the endgame
// let the bishop pair dominate knight+bishop or two knights.
const bishopPairMG = 21
const bishopPairEG = 60

// Rook on open/semi-open file bonuses.
// Open file (no pawns at all): bigger bonus since the rook has full
// penetration potential.  Semi-open (no own pawn, enemy pawn present):
// smaller bonus; the rook pressures the enemy pawn but is partly blocked.
// EG values are near-zero: open files drive MG tactics, not endgame play.
const rookOpenFileMG = 31
const rookOpenFileEG = 3
const rookSemiOpenFileMG = 20
const rookSemiOpenFileEG = -1

// Pawn weaknesses
const isolatedMG = -10
const isolatedEG = -18
const isolatedOpenMG = -9
const backwardMG = 0
const backwardEG = -5
const backwardOpenMG = -4

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
	0: {0, 19, 23, 7, 35, 91, 184, 0}, // free
	1: {0, 20, 29, -6, 18, 36, 78, 0}, // blocked
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
var kingAttackerWeight = [6]int{0, 65, 78, 44, -29, 0}

// King-safety weights used in evaluateKing.
// These are package-level vars so the tuner can read/write them.
var (
	safeCheckWeight   = [4]int{143, 14, 56, 38} // N, B, R, Q safe check weights
	weakInRingWeight  = -19
	queenContactBonus = 87
	noQueenMul        = 3
	noQueenDiv        = 8
	dangerEgDiv       = 4
)

// Pawn shield penalties (MG only).
var (
	shieldMissing  = 11
	shieldAdvanced = 7
	shieldOpenFile = 39
	shieldSemiOpen = 19
)

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

	// Pawn attacks
	e.attackedBy2[White] = doubleWPAttacks(p.pieceBB(White, P))
	e.attackedBy2[Black] = doubleBPAttacks(p.pieceBB(Black, P))
	e.attackedBy[White][P] = shiftWPAttacks(p.pieceBB(White, P))
	e.attackedBy[Black][P] = shiftBPAttacks(p.pieceBB(Black, P))
	e.attacked[White] = e.attackedBy[White][P]
	e.attacked[Black] = e.attackedBy[Black][P]

	// King rings must be set before evaluatePieces so that attack
	// tracking against the enemy king zone is available.
	e.kingRing[White] = kingAtk[p.kingSq[White]]
	e.kingRing[Black] = kingAtk[p.kingSq[Black]]

	evaluatePieces(p, &e, White)
	evaluatePieces(p, &e, Black)
	evaluatePawnStructure(p, &e)
	evaluatePassers(p, &e, White)
	evaluatePassers(p, &e, Black)
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

	// Pull score of drawish endgames closer to 0
	if e.phase < 6 { // R+R+B = 5
		weight := 100
		if score > 0 {
			weight = getDrawishness(p, White, Black)
		} else if score < 0 {
			weight = getDrawishness(p, Black, White)
		}
		score *= weight
		score /= 100
	}

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

	// Knight eval
	pieces := p.pieceBB(side, N)
	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[N], pieceValEG[N])
		addPST(e, side, N, sq)

		// knight board control
		atks := knightAtk[sq]
		e.addAttacks(side, N, atks)

		// knight mobility
		mob := popCount(atks&^p.colorBB[side]) - mobOffset[N]
		add(e, side, EvalMobility, mobMG[N]*mob, mobEG[N]*mob)

		// knight attacks enemy king
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

	// Bishop eval
	pieces = p.pieceBB(side, B)
	if popCount(pieces) >= 2 {
		add(e, side, EvalOther, bishopPairMG, bishopPairEG)
	}
	for pieces != 0 {
		sq := lsb(pieces)

		// bishop mobility and pst tables
		add(e, side, EvalMaterial, pieceValMG[B], pieceValEG[B])
		addPST(e, side, B, sq)

		// bishop board control
		atks := bishopAttacks(occForBishop, sq)
		e.addAttacks(side, B, bishopAttacks(occ, sq))

		// bishop mobility
		mob := popCount(atks) - mobOffset[B]
		add(e, side, EvalMobility, mobMG[B]*mob, mobEG[B]*mob)

		// bishop attacks enemy king
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

	// Rook eval
	pieces = p.pieceBB(side, R)
	for pieces != 0 {
		sq := lsb(pieces)

		// rook material and pst
		add(e, side, EvalMaterial, pieceValMG[R], pieceValEG[R])
		addPST(e, side, R, sq)

		// rook board control
		atks := rookAttacks(occForRook, sq)
		e.addAttacks(side, R, rookAttacks(occ, sq))

		// rook mobility
		mob := popCount(atks) - mobOffset[R]
		add(e, side, EvalMobility, mobMG[R]*mob, mobEG[R]*mob)

		// rook attacks enemy king
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

	// Queen eval
	pieces = p.pieceBB(side, Q)
	for pieces != 0 {
		sq := lsb(pieces)

		// queen material and pst
		add(e, side, EvalMaterial, pieceValMG[Q], pieceValEG[Q])
		addPST(e, side, Q, sq)

		// queen square control
		atks := queenAttacks(occForQueen, sq)
		e.addAttacks(side, Q, queenAttacks(occ, sq))

		// queen mobility
		mob := popCount(atks) - mobOffset[Q]
		add(e, side, EvalMobility, mobMG[Q]*mob, mobEG[Q]*mob)

		// queen attacks enemy king
		if ringAtks := atks & enemyRing; ringAtks != 0 {
			e.attackWt[side] += kingAttackerWeight[Q]
			e.attackCnt[side] += popCount(ringAtks)
		}

		e.phase += 4
		pieces &= pieces - 1
	}
}

func evaluatePawnStructure(p *Pos, e *EvalData) {

	var key = getPawnKey(p)

	if wscoreMG, bscoreMG, wscoreEG, bscoreEG, ok := probePawnHash(key); ok {
		add(e, White, EvalPawns, wscoreMG, wscoreEG)
		add(e, Black, EvalPawns, bscoreMG, bscoreEG)
	} else {
		evaluatePawns(p, e, White)
		evaluatePawns(p, e, Black)
		storePawnHash(key, e.mgScore[White][EvalPawns], e.mgScore[Black][EvalPawns],
			e.egScore[White][EvalPawns], e.egScore[Black][EvalPawns])
	}
}

// evaluatePawns evaluates pawn structure
//
// - king's pawn shield
// - pawn phalanxes
// - isolated pawns
// - backward pawns
// - doubled pawns
func evaluatePawns(p *Pos, e *EvalData, side int) {

	// Pawn shield only matters in the middlegame.
	shieldMG := pawnShieldMG(p, side)
	add(e, side, EvalPawns, shieldMG, 0)

	var pushSq int
	pieces := p.pieceBB(side, P)

	for pieces != 0 {
		sq := lsb(pieces)
		b := squareBit(sq)

		// Pawn phalanx: two pawns standing side by side.
		if shiftSides(b)&p.pieceBB(side, P) > 0 {
			addPhalanx(e, side, sq)
		}

		frontMask := fillForward(b, side)
		isOpen := frontMask&p.pieceBB(side, P) == 0

		// Isolated pawn: no friendly pawns on adjacent files.
		if adjFileMask[fileOf(sq)]&p.pieceBB(side, P) == 0 {
			add(e, side, EvalPawns, isolatedMG, isolatedEG)
			if isOpen {
				add(e, side, EvalPawns, isolatedOpenMG, 0)
			}
			// Backward pawn
		} else if supportMask[side][sq]&p.pieceBB(side, P) == 0 {
			add(e, side, EvalPawns, backwardMG, backwardEG)
			if isOpen {
				add(e, side, EvalPawns, backwardOpenMG, 0)
			}
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

// evaluatePassers scores the passed pawns for one side.
//
//	Passed pawn cannot be blocked or captured on the same
//  or adjacent file ahead of it.  The bonus grows with
//	rank; a pawn on the 7th rank is almost a queen.

func evaluatePassers(p *Pos, e *EvalData, side int) {

	pieces := p.pieceBB(side, P)

	for pieces != 0 {
		sq := lsb(pieces)
		add(e, side, EvalMaterial, pieceValMG[P], pieceValEG[P])
		addPST(e, side, P, sq)

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
		pieces &= pieces - 1
	}
}

// pawnShieldMG computes the middlegame pawn-shield penalty for a king.
// We inspect the two ranks directly in front of the king on its file
// and the two adjacent files.  Missing pawns and open/semi-open files
// near the king are penalised.
func pawnShieldMG(p *Pos, side int) int {
	// if tunerDisableKingSafety {
	// 	return 0
	// }
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
			penalty += shieldMissing // no shield pawn at all
		} else if !hasPawnR1 {
			penalty += shieldAdvanced // pawn advanced one step
		}

		// Additional penalty for open / semi-open files through the king zone.
		if fileMask&ownPawns == 0 {
			if fileMask&enemyPawns == 0 {
				penalty += shieldOpenFile // open file
			} else {
				penalty += shieldSemiOpen // semi-open file
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

	// if tunerDisableKingSafety {
	// 	return
	// }

	// King-attack danger: pressure accumulated by the *enemy* on our
	// king ring.  We only trigger this when at least two distinct pieces
	// are bearing down on the king zone; a lone attacker is rarely fatal.
	enemy := opp(side)
	if e.attackCnt[enemy] >= 2 {
		// Scale danger by weight and count; kept intentionally modest so
		// the engine does not become reckless about piece sacrifices.
		danger := e.attackWt[enemy] * (e.attackCnt[enemy] + 2) / 8

		occ := p.occupied()

		// notOurDefense: squares not protected by our pieces, excluding
		// our king's coverage since the king may be forced to move anyway.
		notOurDefense := ^(e.attacked[side] &^ e.attackedBy[side][K])

		// Weak squares in king zone: squares the enemy attacks that we
		// don't cover — each is a potential entry point for an attacker.
		weakInRing := e.kingRing[side] & e.attacked[enemy] & notOurDefense

		// Safe checks: enemy pieces that can reach a checking square
		// that is not defended by us — more precise than virtual checks
		// since we only count checks the attacker can safely execute.
		safeForEnemy := ^p.colorBB[enemy] & notOurDefense
		safeKnightChecks := popCount(e.attackedBy[enemy][N] & knightAtk[sq] & safeForEnemy)
		safeBishopChecks := popCount(e.attackedBy[enemy][B] & bishopAttacks(occ, sq) & safeForEnemy)
		safeRookChecks := popCount(e.attackedBy[enemy][R] & rookAttacks(occ, sq) & safeForEnemy)
		safeQueenChecks := popCount(e.attackedBy[enemy][Q] & queenAttacks(occ, sq) & safeForEnemy)

		danger += popCount(weakInRing) * weakInRingWeight
		danger += safeKnightChecks*safeCheckWeight[0] + safeBishopChecks*safeCheckWeight[1] + safeRookChecks*safeCheckWeight[2] + safeQueenChecks*safeCheckWeight[3]

		// Queen contact check: enemy queen can land on a square in our king
		// ring that is supported by enemy pieces but not defended by our
		// non-queen pieces.
		enemySupport := e.attackedBy[enemy][P] | e.attackedBy[enemy][N] |
			e.attackedBy[enemy][B] | e.attackedBy[enemy][R]
		ourDefense := e.attackedBy[side][P] | e.attackedBy[side][N] |
			e.attackedBy[side][B] | e.attackedBy[side][R] | e.attackedBy[side][Q]
		if e.kingRing[side]&e.attackedBy[enemy][Q]&enemySupport & ^ourDefense != 0 {
			danger += queenContactBonus
		}

		// No-queen discount: an attacking force without a queen is far
		// less likely to deliver mate; scale danger down sharply.
		if p.pieceBB(enemy, Q) == 0 {
			danger = danger * noQueenMul / noQueenDiv
		}

		// Apply mostly to MG; small residual in EG so the eval is not
		// completely blind to king safety once queens are traded off.
		add(e, side, EvalSafety, -danger, -danger/dangerEgDiv)
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

func addPhalanx(e *EvalData, side, sq int) {
	add(e, side, EvalPawns, phalanxMgByColor[side][sq], phalanxEgByColor[side][sq])
}

// addPST adds the piece-square table score for a piece on sq.
func addPST(e *EvalData, side, piece, sq int) {
	add(e, side, EvalPst, pstMGByColor[side][piece][sq], pstEGByColor[side][piece][sq])
}

// add adds MG/EG scores for one side to EvalData.
func add(e *EvalData, side int, component EvalComponent, mg, eg int) {
	e.mgScore[side][component] += mg
	e.egScore[side][component] += eg
}
