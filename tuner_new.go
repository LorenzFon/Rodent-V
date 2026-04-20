package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ------------------------------------------------------------
// Extracted-feature gradient descent tuner for full [6][64] PST.
// Requires access to the following elements of the engine:
//
//   - type Pos
//   - parseFEN(*Pos, string)
//   - eval_internal(*Pos, bool) int
//   - constants White, Black, P, N, B, R, Q, K
//   - methods p.pieceBB(side, piece int) uint64
//   - lsb(uint64) int
//
// IMPORTANT:
//   1. ALL PST code must be DISABLED inside eval_internal(),
//      otherwise BaseScore will double-count PST. You can do it
//      by commenting out the body of addPst function.
//   2. This tunes White-oriented PSTs, mirroring Black squares by sq^56.
// ------------------------------------------------------------

var texelK = 1.335

// Auto-fit Texel K (sigmoid slope) at the start of each tune.
// This improves calibration when the tuned parameter set changes.
const autoFitTexelK = true

// Float working PSTs.
var pstMGf [6][64]float64
var pstEGf [6][64]float64

// CoeffEntry stores a single non-zero feature coefficient (sparse representation).
// Index = piece*64 + square (0-383).  Value is the net White-minus-Black count for
// that (piece, square) pair, almost always +1 or -1.
type CoeffEntry struct {
	Index int16
	Value int8
}

// ------------------------------------------------------------
// King Safety Tuner Parameters (float64 for gradient descent)
// ------------------------------------------------------------

// Number of king-safety tunable parameters.
const numKingSafetyParams = 14

// Indices into the kingSafetyF parameter array.
const (
	ksAttWtN         = iota // kingAttackerWeight for N
	ksAttWtB                // kingAttackerWeight for B
	ksAttWtR                // kingAttackerWeight for R
	ksAttWtQ                // kingAttackerWeight for Q
	ksSafeN                 // safe knight check weight
	ksSafeB                 // safe bishop check weight
	ksSafeR                 // safe rook check weight
	ksSafeQ                 // safe queen check weight
	ksWeakRing              // weak squares in ring weight
	ksQueenContact          // queen contact check bonus
	ksShieldMissing         // pawn shield: no pawn
	ksShieldAdvanced        // pawn shield: pawn advanced
	ksShieldOpen            // pawn shield: open file
	ksShieldSemi            // pawn shield: semi-open file
)

// Float working king-safety params, initialized from engine vars.
var kingSafetyF [numKingSafetyParams]float64

// Adam state for king-safety params.
var (
	adamMomKS [numKingSafetyParams]float64
	adamVelKS [numKingSafetyParams]float64
)

// KingSafetyData stores the raw feature counts for one position
// needed to compute king-safety score.  Features are extracted
// as (White's king danger, Black's king danger) and shield counts.
type KingSafetyData struct {
	// Per side: features about the *enemy* attacking *our* king.
	// Index 0 = features for White's king, 1 = Black's king.
	AttackGate   [2]bool       // attackCnt >= 2 gate
	RingScale    [2]float64    // (attackCnt + 2) / 8
	RingAttCount [2][4]float64 // enemy pieces of type N,B,R,Q attacking our ring (count of pieces, not squares)
	SafeChecks   [2][4]float64 // safe check counts: N,B,R,Q
	WeakInRing   [2]float64    // popCount of weak squares in ring
	QueenContact [2]float64    // 0 or 1: queen contact check
	NoQueen      [2]bool       // enemy has no queen (discount flag)

	// Pawn shield features per side (MG only).
	ShieldMissing  [2]float64
	ShieldAdvanced [2]float64
	ShieldOpen     [2]float64
	ShieldSemi     [2]float64
}

// ------------------------------------------------------------
// Threat Tuner Parameters (float64 for gradient descent)
// ------------------------------------------------------------

// Number of threat tunable features (per phase).
// Layout: pawnThreat[5] + knight[2*5] + bishop[2*5] + rook[2*5] +
//
//	queen[2*5] + king[4] + push[1] = 50
const numThreatFeatures = 50

// Feature index bases.
const (
	thPawnBase   = 0 // victim P..Q (5)
	thKnightBase = 5 // [hanging=0][P..Q] (5), [defended=1][P..Q] (5)
	thBishopBase = 15
	thRookBase   = 25
	thQueenBase  = 35
	thKingBase   = 45 // victim P..R (4)
	thPush       = 49
)

// Float working threat params (MG and EG separate).
var (
	threatMGf [numThreatFeatures]float64
	threatEGf [numThreatFeatures]float64
)

// Adam state for threat params.
var (
	adamMomThreatMG [numThreatFeatures]float64
	adamVelThreatMG [numThreatFeatures]float64
	adamMomThreatEG [numThreatFeatures]float64
	adamVelThreatEG [numThreatFeatures]float64
)

// Initial values for delta printing.
var threatMGinitial [numThreatFeatures]float64
var threatEGinitial [numThreatFeatures]float64

// ThreatData stores per-side threat feature counts for one position.
type ThreatData struct {
	Counts [2][numThreatFeatures]float64
}

// TuneData holds everything needed to evaluate one position during tuning.
// Using a sparse Coeffs slice instead of a dense [6][64]int array cuts memory
// ~50x (typical position has ~16 non-zero entries vs 384 slots).
type TuneData struct {
	Result    float64
	BaseScore float64
	Phase     int
	Coeffs    []CoeffEntry
	KS        KingSafetyData
	Threats   ThreatData
}

// All epds
var epdLines []string
var newTunerLoaded bool
var currentTunerFile string

// ------------------------------------------------------------
// Loader
// ------------------------------------------------------------

func loadNewTunerFile() {
	loadTunerFileByName("quiet-labeled.epd")
}

func loadTunerFileByName(filename string) {
	if newTunerLoaded && currentTunerFile == filename {
		return
	}
	// Reset state if switching files.
	newTunerLoaded = false
	epdLines = nil

	epdFile, err := os.Open(filename)
	fmt.Printf("reading epd file '%s' (%v)\n", filename, err == nil)
	if err != nil {
		fmt.Println("Epd file not found!")
		return
	}
	currentTunerFile = filename
	defer epdFile.Close()

	scanner := bufio.NewScanner(epdFile)
	readCnt := 0

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		readCnt++

		if readCnt%1000000 == 0 {
			fmt.Printf("%d positions loaded\n", readCnt)
		}

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
	currentTunerFile = ""
}

// ------------------------------------------------------------
// Data extraction
// ------------------------------------------------------------

// parseResultFromLine auto-detects the file format and returns a result in [0,1].
// Book format: line ends with a bracketed float, e.g. "[0.0]", "[0.5]", "[1.0]".
// EPD format:  result is embedded as the string "1-0", "0-1", or "1/2-1/2".
func parseResultFromLine(line string) float64 {
	// Book format detection: look for trailing [score].
	if lb := strings.LastIndex(line, "["); lb != -1 {
		if rb := strings.LastIndex(line, "]"); rb > lb {
			if score, err := strconv.ParseFloat(strings.TrimSpace(line[lb+1:rb]), 64); err == nil {
				return score
			}
		}
	}
	// EPD format detection.
	if strings.Contains(line, "1-0") {
		return 1.0
	} else if strings.Contains(line, "0-1") {
		return 0.0
	}
	return 0.5
}

func extracTuneData(lines []string) []TuneData {
	var p Pos
	out := make([]TuneData, 0, len(lines))

	for _, line := range lines {
		parseFEN(&p, line)

		result := parseResultFromLine(line)

		baseScore := eval_internal(&p, false)
		if p.side == Black {
			baseScore = -baseScore
		}

		// Build EvalData to extract king safety features.
		// We need attack maps from evaluatePieces, so replicate the
		// eval pipeline partially.
		var e EvalData
		e.attackedBy2[White] = doubleWPAttacks(p.pieceBB(White, P))
		e.attackedBy2[Black] = doubleBPAttacks(p.pieceBB(Black, P))
		e.attackedBy[White][P] = shiftWPAttacks(p.pieceBB(White, P))
		e.attackedBy[Black][P] = shiftBPAttacks(p.pieceBB(Black, P))
		e.attacked[White] = e.attackedBy[White][P]
		e.attacked[Black] = e.attackedBy[Black][P]
		e.kingRing[White] = kingAtk[p.kingSq[White]]
		e.kingRing[Black] = kingAtk[p.kingSq[Black]]
		evaluatePieces(&p, &e, White)
		evaluatePieces(&p, &e, Black)
		// King addAttacks (needed for proper attack maps).
		e.addAttacks(White, K, kingAtk[p.kingSq[White]])
		e.addAttacks(Black, K, kingAtk[p.kingSq[Black]])

		var rec TuneData
		rec.Result = result
		rec.BaseScore = float64(baseScore)
		rec.Phase = extractPhase(line)
		rec.Coeffs = extractFeaturesSparse(&p)
		rec.KS = extractKingSafetyFeatures(&p, &e)
		rec.Threats = extractThreatFeatures(&p, &e)

		out = append(out, rec)
	}

	return out
}

func extractPhase(fen string) int {
	phase := 0

	board := fen
	for i := 0; i < len(board); i++ {
		if board[i] == ' ' {
			board = board[:i]
			break
		}
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

// extractFeaturesSparse returns one CoeffEntry per piece on the board.
// White pieces contribute +1, Black pieces contribute -1 (mirrored by sq^56).
// Using a slice instead of [6][64]int saves ~50x memory for typical positions.
func extractFeaturesSparse(p *Pos) []CoeffEntry {
	coeffs := make([]CoeffEntry, 0, 32)
	for piece := P; piece <= K; piece++ {
		for bb := p.pieceBB(White, piece); bb != 0; bb &= bb - 1 {
			sq := lsb(bb)
			coeffs = append(coeffs, CoeffEntry{int16(piece*64 + sq), 1})
		}
		for bb := p.pieceBB(Black, piece); bb != 0; bb &= bb - 1 {
			sq := lsb(bb) ^ 56
			coeffs = append(coeffs, CoeffEntry{int16(piece*64 + sq), -1})
		}
	}
	return coeffs
}

// extractKingSafetyFeatures mirrors the logic of evaluateKing and pawnShieldMG
// to extract raw feature counts for tuning.  Must be called after evaluatePieces
// has populated e.attackWt, e.attackCnt, e.attackedBy, e.attacked, e.kingRing.
func extractKingSafetyFeatures(p *Pos, e *EvalData) KingSafetyData {
	var ks KingSafetyData
	occ := p.occupied()

	for side := White; side <= Black; side++ {
		enemy := opp(side)
		sq := p.kingSq[side]

		// --- Pawn shield features ---
		kFile := fileOf(sq)
		kRank := rankOf(sq)
		ownPawns := p.pieceBB(side, P)
		enemyPawns := p.pieceBB(enemy, P)

		for df := -1; df <= 1; df++ {
			f := kFile + df
			if f < 0 || f > 7 {
				continue
			}
			fileMask := fileABB << uint(f)
			var r1, r2 int
			if side == White {
				r1, r2 = kRank+1, kRank+2
			} else {
				r1, r2 = kRank-1, kRank-2
			}
			hasPawnR1 := r1 >= 0 && r1 <= 7 && ownPawns&squareBit(makeSquare(f, r1)) != 0
			hasPawnR2 := r2 >= 0 && r2 <= 7 && ownPawns&squareBit(makeSquare(f, r2)) != 0

			if !hasPawnR1 && !hasPawnR2 {
				ks.ShieldMissing[side]++
			} else if !hasPawnR1 {
				ks.ShieldAdvanced[side]++
			}
			if fileMask&ownPawns == 0 {
				if fileMask&enemyPawns == 0 {
					ks.ShieldOpen[side]++
				} else {
					ks.ShieldSemi[side]++
				}
			}
		}

		// --- King attack features ---
		if e.attackCnt[enemy] < 2 {
			continue
		}
		ks.AttackGate[side] = true
		ks.RingScale[side] = float64(e.attackCnt[enemy]+2) / 8.0

		// Per-piece ring attacker counts (number of *pieces*, not squares).
		for bb := p.pieceBB(enemy, N); bb != 0; bb &= bb - 1 {
			if knightAtk[lsb(bb)]&e.kingRing[side] != 0 {
				ks.RingAttCount[side][0]++
			}
		}
		for bb := p.pieceBB(enemy, B); bb != 0; bb &= bb - 1 {
			if bishopAttacks(occ, lsb(bb))&e.kingRing[side] != 0 {
				ks.RingAttCount[side][1]++
			}
		}
		for bb := p.pieceBB(enemy, R); bb != 0; bb &= bb - 1 {
			if rookAttacks(occ, lsb(bb))&e.kingRing[side] != 0 {
				ks.RingAttCount[side][2]++
			}
		}
		for bb := p.pieceBB(enemy, Q); bb != 0; bb &= bb - 1 {
			if queenAttacks(occ, lsb(bb))&e.kingRing[side] != 0 {
				ks.RingAttCount[side][3]++
			}
		}

		// Safe checks.
		notOurDefense := ^(e.attacked[side] &^ e.attackedBy[side][K])
		safeForEnemy := ^p.colorBB[enemy] & notOurDefense
		ks.SafeChecks[side][0] = float64(popCount(e.attackedBy[enemy][N] & knightAtk[sq] & safeForEnemy))
		ks.SafeChecks[side][1] = float64(popCount(e.attackedBy[enemy][B] & bishopAttacks(occ, sq) & safeForEnemy))
		ks.SafeChecks[side][2] = float64(popCount(e.attackedBy[enemy][R] & rookAttacks(occ, sq) & safeForEnemy))
		ks.SafeChecks[side][3] = float64(popCount(e.attackedBy[enemy][Q] & queenAttacks(occ, sq) & safeForEnemy))

		// Weak squares in ring.
		weakInRing := e.kingRing[side] & e.attacked[enemy] & notOurDefense
		ks.WeakInRing[side] = float64(popCount(weakInRing))

		// Queen contact check.
		enemySupport := e.attackedBy[enemy][P] | e.attackedBy[enemy][N] |
			e.attackedBy[enemy][B] | e.attackedBy[enemy][R]
		ourDefense := e.attackedBy[side][P] | e.attackedBy[side][N] |
			e.attackedBy[side][B] | e.attackedBy[side][R] | e.attackedBy[side][Q]
		if e.kingRing[side]&e.attackedBy[enemy][Q]&enemySupport & ^ourDefense != 0 {
			ks.QueenContact[side] = 1.0
		}

		// No-queen flag.
		if p.pieceBB(enemy, Q) == 0 {
			ks.NoQueen[side] = true
		}
	}
	return ks
}

// extractThreatFeatures mirrors evaluateThreats to extract per-side threat
// feature counts.  Must be called after evaluatePieces + king addAttacks
// have fully populated e.attackedBy, e.attackedBy2, e.attacked.
func extractThreatFeatures(p *Pos, e *EvalData) ThreatData {
	var td ThreatData
	for side := White; side <= Black; side++ {
		enemy := opp(side)
		enemyPieces := p.colorBB[enemy]

		defendedBB := e.attackedBy2[enemy] |
			e.attackedBy[enemy][P] |
			(e.attacked[enemy] &^ e.attackedBy2[side])

		// Pawn threats.
		for bb := e.attackedBy[side][P] & enemyPieces; bb != 0; {
			sq := lsb(bb)
			bb &= bb - 1
			victim := p.typeAt(sq)
			if victim <= Q {
				td.Counts[side][thPawnBase+victim]++
			}
		}

		// Minor/major piece threats with defended flag.
		type attackerSpec struct {
			piece int
			base  int
		}
		for _, as := range []attackerSpec{{N, thKnightBase}, {B, thBishopBase}, {R, thRookBase}, {Q, thQueenBase}} {
			threats := e.attackedBy[side][as.piece] & enemyPieces
			if as.piece == Q {
				threats &^= p.pieceBB(enemy, K)
			}
			for bb := threats; bb != 0; {
				sq := lsb(bb)
				bb &= bb - 1
				victim := p.typeAt(sq)
				if victim <= Q {
					def := 0
					if defendedBB&squareBit(sq) != 0 {
						def = 1
					}
					td.Counts[side][as.base+def*5+victim]++
				}
			}
		}

		// King threats (undefended only, victims P..R).
		for bb := e.attackedBy[side][K] & enemyPieces &^ defendedBB; bb != 0; {
			sq := lsb(bb)
			bb &= bb - 1
			victim := p.typeAt(sq)
			if victim <= R {
				td.Counts[side][thKingBase+victim]++
			}
		}

		// Push threats.
		occ := p.occupied()
		ownPawns := p.pieceBB(side, P)
		nonPawnEnemies := enemyPieces &^ p.pieceBB(enemy, P)
		enemyPawnAtks := e.attackedBy[enemy][P]
		var pushes uint64
		if side == White {
			pushes = (ownPawns << 8) &^ occ
			pushes |= ((pushes & rank3BB) << 8) &^ occ
		} else {
			pushes = (ownPawns >> 8) &^ occ
			pushes |= ((pushes & rank6BB) >> 8) &^ occ
		}
		safePushes := pushes &^ enemyPawnAtks
		var pushThreatBB uint64
		if side == White {
			pushThreatBB = ((safePushes << 7) &^ fileHBB) | ((safePushes << 9) &^ fileABB)
		} else {
			pushThreatBB = ((safePushes >> 7) &^ fileHBB) | ((safePushes >> 9) &^ fileABB)
		}
		td.Counts[side][thPush] = float64(popCount(pushThreatBB & nonPawnEnemies))
	}
	return td
}

// ------------------------------------------------------------
// Scoring and loss
// ------------------------------------------------------------

// scoreThreatsSide returns (mg, eg) threat score for one side.
func scoreThreatsSide(td *ThreatData, side int) (float64, float64) {
	mg := 0.0
	eg := 0.0
	for k := 0; k < numThreatFeatures; k++ {
		mg += td.Counts[side][k] * threatMGf[k]
		eg += td.Counts[side][k] * threatEGf[k]
	}
	return mg, eg
}

// scoreKingSafetySide computes the king-safety danger score for one side
// using the tuner's float parameters.  Returns (mgContrib, egContrib)
// from that side's perspective (negative = bad for that side).
func scoreKingSafetySide(ks *KingSafetyData, side int) (float64, float64) {
	// Pawn shield (MG only, always active).
	shieldPenalty := kingSafetyF[ksShieldMissing]*ks.ShieldMissing[side] +
		kingSafetyF[ksShieldAdvanced]*ks.ShieldAdvanced[side] +
		kingSafetyF[ksShieldOpen]*ks.ShieldOpen[side] +
		kingSafetyF[ksShieldSemi]*ks.ShieldSemi[side]

	if !ks.AttackGate[side] {
		return -shieldPenalty, 0.0
	}

	// Danger from enemy attacks.
	attackWt := 0.0
	for i := 0; i < 4; i++ {
		attackWt += kingSafetyF[ksAttWtN+i] * ks.RingAttCount[side][i]
	}
	danger := attackWt * ks.RingScale[side]

	for i := 0; i < 4; i++ {
		danger += kingSafetyF[ksSafeN+i] * ks.SafeChecks[side][i]
	}
	danger += kingSafetyF[ksWeakRing] * ks.WeakInRing[side]
	danger += kingSafetyF[ksQueenContact] * ks.QueenContact[side]

	if ks.NoQueen[side] {
		danger = danger * 3.0 / 8.0
	}

	mgSafety := -danger - shieldPenalty
	egSafety := -danger / 4.0
	return mgSafety, egSafety
}

func scoreEntry(td *TuneData) float64 {
	mg := 0.0
	eg := 0.0
	for _, c := range td.Coeffs {
		f := float64(c.Value)
		piece := int(c.Index >> 6) // c.Index / 64
		sq := int(c.Index & 63)    // c.Index % 64
		mg += f * pstMGf[piece][sq]
		eg += f * pstEGf[piece][sq]
	}

	// King safety contribution (White - Black).
	wMG, wEG := scoreKingSafetySide(&td.KS, White)
	bMG, bEG := scoreKingSafetySide(&td.KS, Black)
	mg += wMG - bMG
	eg += wEG - bEG

	// Threat contribution (White - Black).
	wThrMG, wThrEG := scoreThreatsSide(&td.Threats, White)
	bThrMG, bThrEG := scoreThreatsSide(&td.Threats, Black)
	mg += wThrMG - bThrMG
	eg += wThrEG - bThrEG

	tuneScore := (mg*float64(td.Phase) + eg*float64(24-td.Phase)) / 24.0
	return td.BaseScore + tuneScore
}

func texelSigmoidFloat(score float64, k float64) float64 {
	exponent := -(k * score / 400.0)
	return 1.0 / (1.0 + math.Pow(10.0, exponent))
}

func texelSigmoidDerivative(score float64, k float64) float64 {
	p := texelSigmoidFloat(score, k)
	return math.Ln10 * k / 400.0 * p * (1.0 - p)
}

func texelFitOnData(data []TuneData) float64 {
	return texelFitOnDataWithK(data, texelK)
}

func texelFitOnDataWithK(data []TuneData, k float64) float64 {
	if len(data) == 0 {
		return 0.0
	}

	total := 0.0
	for i := range data {
		score := scoreEntry(&data[i])
		prediction := texelSigmoidFloat(score, k)
		diff := data[i].Result - prediction
		total += diff * diff
	}
	return total / float64(len(data))
}

// fitTexelK finds a good sigmoid slope for current scores by minimizing
// mean squared error via ternary search on a bounded interval.
func fitTexelK(data []TuneData) float64 {
	if len(data) == 0 {
		return texelK
	}

	left := 0.1
	right := 6.0
	for i := 0; i < 40; i++ {
		m1 := left + (right-left)/3.0
		m2 := right - (right-left)/3.0
		f1 := texelFitOnDataWithK(data, m1)
		f2 := texelFitOnDataWithK(data, m2)
		if f1 < f2 {
			right = m2
		} else {
			left = m1
		}
	}
	return (left + right) / 2.0
}

// ------------------------------------------------------------
// Gradient descent  (parallel workers + Adam optimizer)
// ------------------------------------------------------------

// Adam optimizer state.  Reset at the start of each tuning session.
var (
	adamMomMG [6][64]float64
	adamVelMG [6][64]float64
	adamMomEG [6][64]float64
	adamVelEG [6][64]float64
)

const (
	adamBeta1 = 0.9
	adamBeta2 = 0.999
	adamEps   = 1e-8
	// Adaptive LR: when MSE worsens enough, decay learning rate.
	lrDropEpsilon = 1e-12
	lrDecayFactor = 0.5
	lrMin         = 1e-4
)

// gradientDescentEpoch computes one Adam-optimized epoch using all available CPU
// cores.  Each worker accumulates its own gradient arrays over a data partition;
// the main goroutine merges them and applies the Adam update.
// kingSafetyGradSide computes d(mgSafety)/d(param_k) and d(egSafety)/d(param_k)
// for one side.  The gradient has the same sign convention as scoreKingSafetySide:
// positive param_k increases danger which decreases the score (more negative).
func kingSafetyGradSide(ks *KingSafetyData, side int, gradMG, gradEG *[numKingSafetyParams]float64) {
	gradMG[ksShieldMissing] -= ks.ShieldMissing[side]
	gradMG[ksShieldAdvanced] -= ks.ShieldAdvanced[side]
	gradMG[ksShieldOpen] -= ks.ShieldOpen[side]
	gradMG[ksShieldSemi] -= ks.ShieldSemi[side]

	if !ks.AttackGate[side] {
		return
	}

	// noQueen discount factor.
	nqScale := 1.0
	if ks.NoQueen[side] {
		nqScale = 3.0 / 8.0
	}

	for i := 0; i < 4; i++ {
		dd := ks.RingAttCount[side][i] * ks.RingScale[side]
		gradMG[ksAttWtN+i] -= dd * nqScale
		gradEG[ksAttWtN+i] -= dd * nqScale / 4.0
	}

	for i := 0; i < 4; i++ {
		dd := ks.SafeChecks[side][i]
		gradMG[ksSafeN+i] -= dd * nqScale
		gradEG[ksSafeN+i] -= dd * nqScale / 4.0
	}

	dd := ks.WeakInRing[side]
	gradMG[ksWeakRing] -= dd * nqScale
	gradEG[ksWeakRing] -= dd * nqScale / 4.0

	dd = ks.QueenContact[side]
	gradMG[ksQueenContact] -= dd * nqScale
	gradEG[ksQueenContact] -= dd * nqScale / 4.0
}

func gradientDescentEpoch(data []TuneData, lr float64) float64 {
	numWorkers := runtime.NumCPU()
	if numWorkers > len(data) {
		numWorkers = len(data)
	}

	type workerResult struct {
		gradMG    [6][64]float64
		gradEG    [6][64]float64
		gradKS    [numKingSafetyParams]float64
		gradThrMG [numThreatFeatures]float64
		gradThrEG [numThreatFeatures]float64
		loss      float64
	}

	results := make([]workerResult, numWorkers)
	chunkSize := (len(data) + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			start := wid * chunkSize
			end := start + chunkSize
			if end > len(data) {
				end = len(data)
			}
			var r workerResult
			for i := start; i < end; i++ {
				td := &data[i]
				score := scoreEntry(td)
				pred := texelSigmoidFloat(score, texelK)
				diff := pred - td.Result
				r.loss += diff * diff

				dLoss := 2.0 * diff * texelSigmoidDerivative(score, texelK)
				mgFactor := float64(td.Phase) / 24.0
				egFactor := float64(24-td.Phase) / 24.0

				for _, c := range td.Coeffs {
					f := float64(c.Value)
					piece := int(c.Index >> 6)
					sq := int(c.Index & 63)
					r.gradMG[piece][sq] += dLoss * f * mgFactor
					r.gradEG[piece][sq] += dLoss * f * egFactor
				}

				// King safety gradients: score += wSafety - bSafety.
				var ksGradMG, ksGradEG [numKingSafetyParams]float64
				var bGradMG, bGradEG [numKingSafetyParams]float64
				kingSafetyGradSide(&td.KS, White, &ksGradMG, &ksGradEG)
				kingSafetyGradSide(&td.KS, Black, &bGradMG, &bGradEG)
				for k := 0; k < numKingSafetyParams; k++ {
					dMG := ksGradMG[k] - bGradMG[k]
					dEG := ksGradEG[k] - bGradEG[k]
					r.gradKS[k] += dLoss * (dMG*mgFactor + dEG*egFactor)
				}

				// Threat gradients: score += wThreat - bThreat, each linear in params.
				for k := 0; k < numThreatFeatures; k++ {
					net := td.Threats.Counts[White][k] - td.Threats.Counts[Black][k]
					r.gradThrMG[k] += dLoss * net * mgFactor
					r.gradThrEG[k] += dLoss * net * egFactor
				}
			}
			results[wid] = r
		}(w)
	}
	wg.Wait()

	// Merge worker gradients.
	var totalGradMG, totalGradEG [6][64]float64
	var totalGradKS [numKingSafetyParams]float64
	var totalGradThrMG, totalGradThrEG [numThreatFeatures]float64
	totalLoss := 0.0
	for _, r := range results {
		totalLoss += r.loss
		for piece := P; piece <= K; piece++ {
			for sq := 0; sq < 64; sq++ {
				totalGradMG[piece][sq] += r.gradMG[piece][sq]
				totalGradEG[piece][sq] += r.gradEG[piece][sq]
			}
		}
		for k := 0; k < numKingSafetyParams; k++ {
			totalGradKS[k] += r.gradKS[k]
		}
		for k := 0; k < numThreatFeatures; k++ {
			totalGradThrMG[k] += r.gradThrMG[k]
			totalGradThrEG[k] += r.gradThrEG[k]
		}
	}

	// Adam parameter update.
	scale := 1.0 / float64(len(data))
	for piece := P; piece <= K; piece++ {
		for sq := 0; sq < 64; sq++ {
			gMG := totalGradMG[piece][sq] * scale
			adamMomMG[piece][sq] = adamBeta1*adamMomMG[piece][sq] + (1-adamBeta1)*gMG
			adamVelMG[piece][sq] = adamBeta2*adamVelMG[piece][sq] + (1-adamBeta2)*gMG*gMG
			pstMGf[piece][sq] -= lr * adamMomMG[piece][sq] / (math.Sqrt(adamVelMG[piece][sq]) + adamEps)

			gEG := totalGradEG[piece][sq] * scale
			adamMomEG[piece][sq] = adamBeta1*adamMomEG[piece][sq] + (1-adamBeta1)*gEG
			adamVelEG[piece][sq] = adamBeta2*adamVelEG[piece][sq] + (1-adamBeta2)*gEG*gEG
			pstEGf[piece][sq] -= lr * adamMomEG[piece][sq] / (math.Sqrt(adamVelEG[piece][sq]) + adamEps)
		}
	}

	// Adam update for king safety params.
	for k := 0; k < numKingSafetyParams; k++ {
		g := totalGradKS[k] * scale
		adamMomKS[k] = adamBeta1*adamMomKS[k] + (1-adamBeta1)*g
		adamVelKS[k] = adamBeta2*adamVelKS[k] + (1-adamBeta2)*g*g
		kingSafetyF[k] -= lr * adamMomKS[k] / (math.Sqrt(adamVelKS[k]) + adamEps)
	}

	// Adam update for threat params.
	for k := 0; k < numThreatFeatures; k++ {
		gMG := totalGradThrMG[k] * scale
		adamMomThreatMG[k] = adamBeta1*adamMomThreatMG[k] + (1-adamBeta1)*gMG
		adamVelThreatMG[k] = adamBeta2*adamVelThreatMG[k] + (1-adamBeta2)*gMG*gMG
		threatMGf[k] -= lr * adamMomThreatMG[k] / (math.Sqrt(adamVelThreatMG[k]) + adamEps)

		gEG := totalGradThrEG[k] * scale
		adamMomThreatEG[k] = adamBeta1*adamMomThreatEG[k] + (1-adamBeta1)*gEG
		adamVelThreatEG[k] = adamBeta2*adamVelThreatEG[k] + (1-adamBeta2)*gEG*gEG
		threatEGf[k] -= lr * adamMomThreatEG[k] / (math.Sqrt(adamVelThreatEG[k]) + adamEps)
	}

	return totalLoss * scale
}

func gradientDescentSession(data []TuneData, epochs int, lr float64) {
	// Reset Adam state for a fresh session.
	adamMomMG = [6][64]float64{}
	adamVelMG = [6][64]float64{}
	adamMomEG = [6][64]float64{}
	adamVelEG = [6][64]float64{}
	adamMomKS = [numKingSafetyParams]float64{}
	adamVelKS = [numKingSafetyParams]float64{}
	adamMomThreatMG = [numThreatFeatures]float64{}
	adamVelThreatMG = [numThreatFeatures]float64{}
	adamMomThreatEG = [numThreatFeatures]float64{}
	adamVelThreatEG = [numThreatFeatures]float64{}

	numWorkers := runtime.NumCPU()
	fmt.Printf("Using %d worker goroutines (Adam optimizer)\n", numWorkers)

	if autoFitTexelK {
		oldK := texelK
		texelK = fitTexelK(data)
		fmt.Printf("Auto-fit texelK: %.6f -> %.6f\n", oldK, texelK)
	}

	startLoss := texelFitOnData(data)
	fmt.Printf("Initial loss = %.10f\n", startLoss)
	currentLR := lr
	fmt.Printf("Initial learning rate = %.6f\n", currentLR)

	prevLoss := startLoss
	stall := 0

	for epoch := 0; epoch < epochs; epoch++ {
		loss := gradientDescentEpoch(data, currentLR)
		fmt.Printf("epoch %d  loss = %.10f  lr = %.6f\n", epoch, loss, currentLR)

		// If loss got worse, lower LR to stabilize updates.
		if loss > prevLoss+lrDropEpsilon {
			newLR := currentLR * lrDecayFactor
			if newLR < lrMin {
				newLR = lrMin
			}
			if newLR < currentLR {
				fmt.Printf("  loss increased (prev %.10f -> %.10f), reducing lr: %.6f -> %.6f\n", prevLoss, loss, currentLR, newLR)
				currentLR = newLR
			}
		}
		if epoch%100 == 0 {
			printSnapshot()
		}

		if math.Abs(prevLoss-loss) < 1e-10 {
			stall++
			if stall >= 20 {
				fmt.Println("Stopping: loss no longer changes materially.")
				break
			}
		} else {
			stall = 0
		}
		prevLoss = loss
	}

	finalLoss := texelFitOnData(data)
	fmt.Printf("Final loss = %.10f\n", finalLoss)
}

// ------------------------------------------------------------
// Initialization helpers
// ------------------------------------------------------------

func initFromExistingTables(srcMG, srcEG *[6][64]int) {
	for piece := P; piece <= K; piece++ {
		for sq := 0; sq < 64; sq++ {
			pstMGf[piece][sq] = float64(srcMG[piece][sq])
			pstEGf[piece][sq] = float64(srcEG[piece][sq])
		}
	}
}

func initKingSafetyParams() {
	kingSafetyF[ksAttWtN] = float64(kingAttackerWeight[N])
	kingSafetyF[ksAttWtB] = float64(kingAttackerWeight[B])
	kingSafetyF[ksAttWtR] = float64(kingAttackerWeight[R])
	kingSafetyF[ksAttWtQ] = float64(kingAttackerWeight[Q])
	kingSafetyF[ksSafeN] = float64(safeCheckWeight[0])
	kingSafetyF[ksSafeB] = float64(safeCheckWeight[1])
	kingSafetyF[ksSafeR] = float64(safeCheckWeight[2])
	kingSafetyF[ksSafeQ] = float64(safeCheckWeight[3])
	kingSafetyF[ksWeakRing] = float64(weakInRingWeight)
	kingSafetyF[ksQueenContact] = float64(queenContactBonus)
	kingSafetyF[ksShieldMissing] = float64(shieldMissing)
	kingSafetyF[ksShieldAdvanced] = float64(shieldAdvanced)
	kingSafetyF[ksShieldOpen] = float64(shieldOpenFile)
	kingSafetyF[ksShieldSemi] = float64(shieldSemiOpen)
}

func initThreatParams() {
	// Pawn threats: victim P..Q
	for v := P; v <= Q; v++ {
		threatMGf[thPawnBase+v] = float64(threatByPawnMG[v])
		threatEGf[thPawnBase+v] = float64(threatByPawnEG[v])
	}
	// Knight threats: [hanging/defended][P..Q]
	for def := 0; def <= 1; def++ {
		for v := P; v <= Q; v++ {
			threatMGf[thKnightBase+def*5+v] = float64(threatByKnightMG[def][v])
			threatEGf[thKnightBase+def*5+v] = float64(threatByKnightEG[def][v])
		}
	}
	// Bishop threats
	for def := 0; def <= 1; def++ {
		for v := P; v <= Q; v++ {
			threatMGf[thBishopBase+def*5+v] = float64(threatByBishopMG[def][v])
			threatEGf[thBishopBase+def*5+v] = float64(threatByBishopEG[def][v])
		}
	}
	// Rook threats
	for def := 0; def <= 1; def++ {
		for v := P; v <= Q; v++ {
			threatMGf[thRookBase+def*5+v] = float64(threatByRookMG[def][v])
			threatEGf[thRookBase+def*5+v] = float64(threatByRookEG[def][v])
		}
	}
	// Queen threats
	for def := 0; def <= 1; def++ {
		for v := P; v <= Q; v++ {
			threatMGf[thQueenBase+def*5+v] = float64(threatByQueenMG[def][v])
			threatEGf[thQueenBase+def*5+v] = float64(threatByQueenEG[def][v])
		}
	}
	// King threats: victim P..R
	for v := P; v <= R; v++ {
		threatMGf[thKingBase+v] = float64(threatByKingMG[v])
		threatEGf[thKingBase+v] = float64(threatByKingEG[v])
	}
	// Push threats
	threatMGf[thPush] = float64(pushThreatMG)
	threatEGf[thPush] = float64(pushThreatEG)
}

// ------------------------------------------------------------
// Printing helpers
// ------------------------------------------------------------

func roundToInt(x float64) int {
	if x >= 0 {
		return int(x + 0.5)
	}
	return int(x - 0.5)
}

func printSinglePSTFloatAsInt(label string, table *[64]float64) {
	fmt.Printf("\t%s: {\n", label)
	for row := 0; row < 8; row++ {
		start := row * 8
		fmt.Print("\t\t")
		for i := 0; i < 8; i++ {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%4d", roundToInt(table[start+i]))
		}
		fmt.Println(",")
	}
	fmt.Println("\t},")
}

// print tuned PST converted to int, so that it can be pasted
// directly into source code
func printPSTSetAsInt(varName string, pst *[6][64]float64) {
	fmt.Printf("var %s = [6][64]int{\n", varName)
	for piece := P; piece <= K; piece++ {
		printSinglePSTFloatAsInt(pstLabels[piece], &pst[piece])
	}
	fmt.Println("}")
}

// Optional: print deltas against initial PSTs.
func printPSTDeltaSet(varName string, base *[6][64]int, tuned *[6][64]float64) {
	fmt.Printf("var %s = [6][64]int{\n", varName)
	for piece := P; piece <= K; piece++ {
		fmt.Printf("\t%s: {\n", pstLabels[piece])
		for row := 0; row < 8; row++ {
			start := row * 8
			fmt.Print("\t\t")
			for i := 0; i < 8; i++ {
				if i > 0 {
					fmt.Print(", ")
				}
				d := roundToInt(tuned[piece][start+i]) - base[piece][start+i]
				fmt.Printf("%4d", d)
			}
			fmt.Println(",")
		}
		fmt.Println("\t},")
	}
	fmt.Println("}")
}

var kingSafetyParamNames = [numKingSafetyParams]string{
	"kingAttackerWeight[N]", "kingAttackerWeight[B]",
	"kingAttackerWeight[R]", "kingAttackerWeight[Q]",
	"safeCheckWeight[N]", "safeCheckWeight[B]",
	"safeCheckWeight[R]", "safeCheckWeight[Q]",
	"weakInRingWeight", "queenContactBonus",
	"shieldMissing", "shieldAdvanced",
	"shieldOpenFile", "shieldSemiOpen",
}

func printKingSafetyParams() {
	fmt.Println("\n// King safety tuned parameters:")
	fmt.Printf("var kingAttackerWeight = [6]int{0, %d, %d, %d, %d, 0}\n",
		roundToInt(kingSafetyF[ksAttWtN]), roundToInt(kingSafetyF[ksAttWtB]),
		roundToInt(kingSafetyF[ksAttWtR]), roundToInt(kingSafetyF[ksAttWtQ]))
	fmt.Printf("var safeCheckWeight = [4]int{%d, %d, %d, %d}  // N, B, R, Q\n",
		roundToInt(kingSafetyF[ksSafeN]), roundToInt(kingSafetyF[ksSafeB]),
		roundToInt(kingSafetyF[ksSafeR]), roundToInt(kingSafetyF[ksSafeQ]))
	fmt.Printf("var weakInRingWeight = %d\n", roundToInt(kingSafetyF[ksWeakRing]))
	fmt.Printf("var queenContactBonus = %d\n", roundToInt(kingSafetyF[ksQueenContact]))
	fmt.Printf("var shieldMissing = %d\n", roundToInt(kingSafetyF[ksShieldMissing]))
	fmt.Printf("var shieldAdvanced = %d\n", roundToInt(kingSafetyF[ksShieldAdvanced]))
	fmt.Printf("var shieldOpenFile = %d\n", roundToInt(kingSafetyF[ksShieldOpen]))
	fmt.Printf("var shieldSemiOpen = %d\n", roundToInt(kingSafetyF[ksShieldSemi]))

	fmt.Println("\n// King safety param deltas:")
	for k := 0; k < numKingSafetyParams; k++ {
		fmt.Printf("  %s: %.2f (delta %+.2f)\n", kingSafetyParamNames[k],
			kingSafetyF[k], kingSafetyF[k]-kingSafetyInitial[k])
	}
}

// Store initial values for delta printing.
var kingSafetyInitial [numKingSafetyParams]float64

// printThreatTable6 prints from float array with given offset.
func printThreatTable6(varName string, arr *[numThreatFeatures]float64, base int) {
	fmt.Printf("var %s = [6]int{", varName)
	for v := 0; v < 6; v++ {
		if v > 0 {
			fmt.Print(", ")
		}
		if v < 5 {
			fmt.Printf("%d", roundToInt(arr[base+v]))
		} else {
			fmt.Print("0")
		}
	}
	fmt.Println("}")
}

func printThreatTable26(varName string, arr *[numThreatFeatures]float64, base int) {
	fmt.Printf("var %s = [2][6]int{\n", varName)
	for def := 0; def <= 1; def++ {
		fmt.Print("\t{")
		for v := 0; v < 6; v++ {
			if v > 0 {
				fmt.Print(", ")
			}
			if v < 5 {
				fmt.Printf("%d", roundToInt(arr[base+def*5+v]))
			} else {
				fmt.Print("0")
			}
		}
		fmt.Println("},")
	}
	fmt.Println("}")
}

func printThreatKing(varName string, arr *[numThreatFeatures]float64) {
	fmt.Printf("var %s = [6]int{", varName)
	for v := 0; v < 6; v++ {
		if v > 0 {
			fmt.Print(", ")
		}
		if v <= R {
			fmt.Printf("%d", roundToInt(arr[thKingBase+v]))
		} else {
			fmt.Print("0")
		}
	}
	fmt.Println("}")
}

func printThreatParams() {
	fmt.Println("\n// Threat tuned parameters:")
	printThreatTable6("threatByPawnMG", &threatMGf, thPawnBase)
	printThreatTable6("threatByPawnEG", &threatEGf, thPawnBase)
	printThreatTable26("threatByKnightMG", &threatMGf, thKnightBase)
	printThreatTable26("threatByKnightEG", &threatEGf, thKnightBase)
	printThreatTable26("threatByBishopMG", &threatMGf, thBishopBase)
	printThreatTable26("threatByBishopEG", &threatEGf, thBishopBase)
	printThreatTable26("threatByRookMG", &threatMGf, thRookBase)
	printThreatTable26("threatByRookEG", &threatEGf, thRookBase)
	printThreatTable26("threatByQueenMG", &threatMGf, thQueenBase)
	printThreatTable26("threatByQueenEG", &threatEGf, thQueenBase)
	printThreatKing("threatByKingMG", &threatMGf)
	printThreatKing("threatByKingEG", &threatEGf)
	fmt.Printf("const pushThreatMG = %d\n", roundToInt(threatMGf[thPush]))
	fmt.Printf("const pushThreatEG = %d\n", roundToInt(threatEGf[thPush]))
}

func printSnapshot() {
	fmt.Println()
	printPSTDeltaSet("pstDeltaMG", &pstMG, &pstMGf)
	fmt.Println()
	printPSTDeltaSet("pstDeltaEG", &pstEG, &pstEGf)
	fmt.Println()
	printPSTSetAsInt("pstMG", &pstMGf)
	fmt.Println()
	printPSTSetAsInt("pstEG", &pstEGf)
	printKingSafetyParams()
	printThreatParams()
}

// ------------------------------------------------------------
// Entry point
// ------------------------------------------------------------

func gradientTuneSession(epochs int, lr float64) {
	gradientTuneSessionFromFile("quiet-labeled.epd", epochs, lr)
}

// gradientTuneSessionFromFile runs the tuner on any supported file.
// Supported formats:
//   - quiet-labeled EPD  (result encoded as "1-0" / "0-1" / "1/2-1/2")
//   - lichess book       (result encoded as trailing "[score]" float)
func gradientTuneSessionFromFile(filename string, epochs int, lr float64) {
	// Disable PST, king safety, and threats in eval so BaseScore excludes them.
	tunerDisableKingSafety = true
	tunerDisableThreats = true

	loadTunerFileByName(filename)
	if !newTunerLoaded {
		fmt.Println("tuner data not loaded")
		tunerDisableKingSafety = false
		tunerDisableThreats = false
		return
	}

	data := extracTuneData(epdLines)
	fmt.Printf("Extracted %d positions\n", len(data))

	initFromExistingTables(&pstMG, &pstEG)
	initKingSafetyParams()
	kingSafetyInitial = kingSafetyF
	initThreatParams()
	threatMGinitial = threatMGf
	threatEGinitial = threatEGf
	gradientDescentSession(data, epochs, lr)
	printSnapshot()

	tunerDisableKingSafety = false
	tunerDisableThreats = false
}
