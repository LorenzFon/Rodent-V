// ================================================================
// S11 SEARCH
// ================================================================
//
//   The search is the engine's brain.  Given a position, it finds
//   the best move by recursively exploring the game tree, bounded
//   by alpha-beta pruning.
//
//   ALGORITHM: PRINCIPAL VARIATION SEARCH (PVS)
//   --------------------------------------------
//   PVS is an enhancement of alpha-beta negamax.  The key insight:
//   after finding a move that raises alpha (a "PV move"), every other
//   move can be searched with a zero-width window (alpha, alpha+1)
//   first.  If that search fails high (score > alpha), a re-search
//   with the full window is needed.  In practice, most branches fail
//   the zero-width search immediately, saving significant time.
//   The re-search is triggered whenever score > alpha, with no upper
//   beta guard.
//
//   ITERATIVE DEEPENING + ASPIRATION WINDOWS
//   ------------------------------------------
//   We don't search directly to the maximum depth.  Instead we start
//   at depth 1 and deepen one ply at a time.  Each shallower search
//   populates the TT with good move hints that guide the deeper
//   search.  It also lets us output progress and stop cleanly when
//   time expires.  From depth 4 onward each iteration is searched
//   inside a narrow aspiration window centred on the previous score;
//   on a fail-low or fail-high the window doubles and the search is
//   retried.  This generates many more cutoffs at the root without
//   risking correctness.
//
//   NULL-MOVE PRUNING
//   -----------------
//   If the position is so good that even passing the turn to the
//   opponent (a "null move") at reduced depth still causes a beta
//   cutoff, the branch can be pruned.  The reduction is 3 plies
//   (R=3).  We skip null moves in check (the engine would be in an
//   illegal state) and when only pawns + kings remain (zugzwang risk).
//
//   CHECK EXTENSION
//   ---------------
//   When a move gives check we extend the search by one extra ply.
//   Forcing moves deserve deeper investigation; cutting off a check
//   sequence prematurely can cause catastrophic horizon effect.
//
//   QUIESCENCE SEARCH
//   ------------------
//   At depth 0 we don't immediately return evaluate(). Instead we
//   enter quiesce(), which searches all captures until the position
//   is "quiet" (no captures remain or all are bad).  This prevents
//   the horizon effect where the engine stops just before a piece is
//   taken.  When entered while in check, stand-pat is illegal and the
//   full move pipeline is used so quiet evasions are included; no
//   legal move means checkmate.
//
//   REVERSE FUTILITY PRUNING (RFP)
//   --------------------------------
//   Also called "static null-move pruning".  If the static evaluation
//   already exceeds beta by a depth-scaled margin, the position is so
//   good that even a pessimistic adjustment won't fall below beta.  We
//   return the static eval immediately without searching further.
//   Applied only at non-PV nodes, when not in check, at shallow depth.
//
//   LATE MOVE PRUNING (LMP)
//   -----------------------
//   At shallow depths (depth < 4) on non-PV nodes, quiet moves beyond
//   the first 4*depth+1 are skipped entirely rather than just reduced.
//   quietTried tracks only quiet moves so the threshold is independent
//   of how many captures were searched first.  Moves that give check
//   are exempt: a quiet check can be the only defensive resource or
//   the only way out of a mating attack.  LMP is also skipped when the
//   node itself is in check, since evasions must be fully searched.
//
//   INTERNAL ITERATIVE REDUCTION (IIR)
//   ------------------------------------
//   When no TT move is available, move ordering will be poor and a
//   full-depth search is likely wasteful.  We reduce depth by one ply
//   instead.  The shallower search populates the TT; the next iterative-
//   deepening iteration will find the hint and search at full depth with
//   good ordering.  Applied at depth >= 4; skipped in check since
//   evasions must always be searched at full depth.
//
//   LATE MOVE REDUCTIONS (LMR)
//   --------------------------
//   Moves tried late in the list (after the killer and good captures)
//   are unlikely to be best.  We search them at reduced depth first;
//   if the reduced search surprisingly raises alpha, we re-search at
//   full depth.  The reduction table lmr[depth][moveIndex] is filled
//   once at startup using the formula log(depth)*log(moveIndex)/1.8,
//   clamped to [1, 5].  An extra ply is added when not in a PV node.
//
//   REPETITION DRAW
//   ---------------
//   The engine detects 3-fold repetition by checking the Zobrist key
//   against keyHist.  We look back in steps of 2 (same side to move)
//   up to clock moves (the 50-move clock resets on irreversible moves,
//   so only those positions can repeat).
//

package main

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

// Global search state.
var (
	timeLimit    int64        // allocated move time in ms (-1 = unlimited)
	pondering    bool         // true while in ponder mode (ignore clock)
	rootDepth    int          // current iterative deepening depth
	selDepth     int          // maximum ply reached in the current search
	nodes        int64        // total nodes searched (search-goroutine only; no atomic needed)
	abortFlag    int32        // set to 1 atomically to stop the search
	searchStart  int64        // Unix ms at the start of think()
	rootHistLen  int          // p.histLen at the moment think() began; used by repetition detection
	evalStack    [maxPly]int  // static eval at each ply; noEval sentinel when in check
	contSide     [maxPly]int  // side that made the move at this ply (for cont hist)
	contPiece    [maxPly]int  // piece type (0-5) that moved at this ply
	contTo       [maxPly]int  // destination square at this ply
	contValid    [maxPly]bool // false for null moves and unvisited plies
	excludedMove [maxPly]int  // move excluded during singular extension search (0 = none)
)

// lmr[depth][moveIndex] holds the ply reduction for a quiet move tried
// at position moveIndex in the move list when depth plies remain.
var lmr [64][64]int

func init() {
	initLMRTable()
}

// initLMRTable pre-computes the reduction for every (depth, moveIndex) pair.
// Entries where depth < 3 or moveIndex < 4 are left at zero (no reduction for
// the first few moves or near the leaves).  All other entries use the
// logarithmic formula log(depth)*log(moveIndex)/1.8, rounded and clamped to
// [1, 5] so reductions stay meaningful but never collapse a search entirely.
func initLMRTable() {
	for depth := 0; depth < 64; depth++ {
		for moveIndex := 0; moveIndex < 64; moveIndex++ {
			if depth < 3 || moveIndex < 4 {
				lmr[depth][moveIndex] = 0
				continue
			}

			raw := math.Log(float64(depth)) * math.Log(float64(moveIndex)) / 1.8
			reduction := int(raw + 0.5) // round to nearest

			if reduction < 1 {
				reduction = 1
			} else if reduction > 5 {
				reduction = 5
			}

			lmr[depth][moveIndex] = reduction
		}
	}
}

// think is the top-level search entry point called from the UCI loop.
// It performs iterative deepening from depth 1 to maxDepth, outputting
// UCI info lines and finally "bestmove" when done or time expires.
//
// From depth 5 onward each iteration is searched inside an aspiration window
// centred on the previous score.  The initial delta scales with the score
// magnitude so volatile (unbalanced) positions get a wider starting window.
// On fail-low beta is first collapsed to the midpoint before alpha widens,
// avoiding a needlessly large high-side window.  The delta grows by 50% on
// each failure (smoother than doubling) until the window opens fully.
func think(p *Pos, maxDepth int) {
	ttDate = (ttDate + 1) & 255
	nodes = 0
	selDepth = 0
	atomic.StoreInt32(&abortFlag, 0)
	searchStart = time.Now().UnixMilli()
	rootHistLen = p.histLen
	contValid = [maxPly]bool{} // reset cont hist context; stale entries from prev search must not be used

	var pv [maxPly]int
	score := 0

	for rootDepth = 1; rootDepth <= maxDepth; rootDepth++ {
		// Before starting a new depth, do an unthrottled time check.
		// Two stopping criteria (either is sufficient):
		//   1. Hard limit: elapsed >= timeLimit (don't start if already over).
		//   2. Half-budget rule: elapsed >= timeLimit/2. The next depth
		//      typically takes longer than all prior depths combined, so
		//      starting it when half the budget is gone almost always busts
		//      the limit. This is the key guard against time forfeits.
		if rootDepth > 1 && !pondering && timeLimit >= 0 {
			elapsed := time.Now().UnixMilli() - searchStart
			if elapsed >= timeLimit || elapsed >= timeLimit/2 {
				break
			}
		}

		var iterScore int

		if rootDepth < 5 {
			// Aspiration windows are unreliable at shallow depths.
			iterScore = search(p, 0, -inf, inf, rootDepth, pv[:])
		} else {
			// Score-adaptive initial delta: balanced positions get a tight
			// window; large scores widen it to reduce retry churn.
			delta := 25 + score*score/16384
			alpha := max(-inf, score-delta)
			beta := min(inf, score+delta)

			for {
				iterScore = search(p, 0, alpha, beta, rootDepth, pv[:])
				if atomic.LoadInt32(&abortFlag) != 0 {
					break
				}
				if iterScore <= alpha {
					// Fail low: collapse beta to the midpoint before widening
					// alpha so the high side doesn't grow unnecessarily.
					beta = (alpha + beta) / 2
					alpha = max(-inf, alpha-delta)
				} else if iterScore >= beta {
					// Fail high: widen the window above and retry.
					beta = min(inf, beta+delta)
				} else {
					break // score is inside the window
				}
				delta += delta / 2 // proportional widening (×1.5)
			}
		}

		if atomic.LoadInt32(&abortFlag) != 0 {
			break
		}
		score = iterScore
	}

	if pv[0] != 0 {
		best := moveToStr(pv[0])
		if pv[1] != 0 {
			ponder := moveToStr(pv[1])
			fmt.Printf("bestmove %s ponder %s\n", best, ponder)
		} else {
			fmt.Printf("bestmove %s\n", best)
		}
	} else {
		fmt.Println("bestmove 0000")
	}
}

// search is the recursive alpha-beta negamax function.
//
// Parameters:
//
//	p: current position
//	ply: distance from the root (0 = root)
//	alpha: lower bound on the score we need to improve our line
//	beta: upper bound; a score >= beta causes a cutoff
//	depth: remaining plies to search
//	pv: principal variation output buffer
//
// Returns the score for the side to move (negamax convention).
func search(p *Pos, ply, alpha, beta, depth int, pv []int) int {
	if depth <= 0 {
		return quiesce(p, ply, alpha, beta, pv)
	}

	nodes++
	if ply > selDepth {
		selDepth = ply
	}
	checkTime()
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	isRoot := (ply == 0)

	if !isRoot {
		pv[0] = 0
		// A position repeated from earlier in the game tree is a draw.
		if isRepetition(p) || p.isInsufficientMaterial() {
			checkTime() // sets abort flag - helps in case of many draws in a row
			return 0
		}
	}

	// --- Mate distance pruning ---
	// Tighten the window based on the best/worst mate reachable from this ply.
	// If we already have a proven mate, skip searching slower mates.
	if !isRoot {
		alpha = max(alpha, -mate+ply)
		beta = min(beta, mate-ply-1)
		if alpha >= beta {
			return alpha
		}
	}

	// Are we in the pv node?
	isPv := (beta > alpha+1)
	movesTried := 0

	// --- Transposition table probe ---
	ttMove := 0
	ttFlag := 0
	ttDepth := 0
	score := 0
	if probeTT(p.key, &ttMove, &score, &ttFlag, &ttDepth, alpha, beta, depth, ply) {
		if !isPv && excludedMove[ply] == 0 {
			return score
		}
	}
	ttScore := score // save TT score before NMP/LMR overwrites the score variable

	// Safeguard against hitting max ply limit
	if ply >= maxPly-1 {
		return evaluate(p)
	}

	// Are we in check in this node?
	nodeInCheck := p.inCheck()

	// Cache the static evaluation; shared by RFP, improving, and null-move pruning.
	// Store in evalStack for the improving heuristic (ply-2 comparison).
	// When in check we store a sentinel so the improving check below works correctly.
	staticEval := 0
	if !nodeInCheck {
		staticEval = evaluate(p)
		evalStack[ply] = staticEval
	} else {
		evalStack[ply] = noEval
	}

	// --- Improving heuristic ---
	// "Improving" means our static eval is better than it was two plies ago
	// (our previous turn). When improving we use tighter pruning margins —
	// the position is trending up so we are less likely to collapse below beta.
	// Default to true when no prior eval is available (early plies, post-check).
	improving := true
	if nodeInCheck {
		improving = false
	} else if ply >= 2 && evalStack[ply-2] != noEval {
		improving = staticEval > evalStack[ply-2]
	} else if ply >= 4 && evalStack[ply-4] != noEval {
		improving = staticEval > evalStack[ply-4]
	}

	// --- Reverse futility pruning ---
	// If the static eval beats beta by a depth-scaled margin, the position
	// is already so good that a full search is unlikely to fall below beta.
	// The mate guard prevents pruning when beta is a mate score, where the
	// static eval is unreliable.  We return the margin-adjusted value rather
	// than the raw static eval to avoid inflating the returned score.

	// Use a tighter margin when improving (position is gaining ground).
	// Wider margin when not improving avoids over-pruning a deteriorating line.
	rfpDepthMargin := rfpMargin
	if improving {
		rfpDepthMargin = rfpImpMargin
	}
	if !isPv && !nodeInCheck && depth <= 7 && beta < mate-maxPly &&
		excludedMove[ply] == 0 &&
		staticEval-rfpDepthMargin*depth >= beta {
		return staticEval - rfpDepthMargin*depth
	}

	// --- Razoring ---
	// If static eval is far below alpha at shallow depth, the position is
	// unlikely to improve enough with quiet moves. Drop into qsearch; if
	// qsearch also fails low, return immediately without searching further.
	if !isPv && !nodeInCheck && depth <= 3 &&
		excludedMove[ply] == 0 &&
		staticEval <= alpha-razorMargin*depth {
		s := quiesce(p, ply, alpha, beta, pv)
		if s <= alpha {
			return s
		}
	}

	// --- Null-move pruning ---
	// Skip if: depth <= 1 (too shallow to be reliable), the position
	// is already beyond beta, we are in check, or only pawns remain.
	if depth > 1 && !isPv && !nodeInCheck && p.canNullMove() && beta <= staticEval &&
		excludedMove[ply] == 0 {
		contValid[ply] = false // null move: no valid piece context for cont hist
		reduction := 2 + depth / 6
		var u Undo
		makeNullMove(p, &u)
		var nullPv [maxPly]int
		score = -search(p, ply+1, -beta, -beta+1, depth-1 - reduction, nullPv[:])
		unmakeNullMove(p, &u)
		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}
		if score >= beta {
			return score
		}
	}

	// --- Internal iterative reduction ---
	// Without a TT move the move picker has no good hint; search one
	// ply shallower so the cost of poor ordering is contained.
	// Skipped in check: evasions must be searched at full depth.
	if ttMove == 0 && depth >= 4 && !nodeInCheck {
		depth--
	}

	var bestMove int

	// --- Main move loop ---
	bestScore := -inf
	picker := &moveBuffers[ply]
	initMovePicker(p, picker, ttMove, ply)
	var childPv [maxPly]int

	quietTried := 0
	// quietsMade tracks quiet moves that were fully searched without causing
	// a beta cutoff.  On a cutoff we apply a malus to all of them.
	var quietsMade [maxMoves]int
	quietsMadeCount := 0

	for {
		move, stage := picker.nextMove()
		if move == 0 {
			break
		}

		// Skip the move excluded during a singular extension search.
		if move == excludedMove[ply] {
			continue
		}

		// --- Singular extension ---
		// When the TT move is reliable and deep enough, check if it's the
		// only good move by searching without it at reduced depth. If all
		// other moves fail below a margin, extend this move by +1 ply.
		// Negative extension: if the position isn't singular AND the TT
		// score already beats beta, reduce aggressively (multi-cut signal).
		extension := 0
		if move == ttMove &&
			excludedMove[ply] == 0 &&
			depth >= 8 &&
			ttDepth >= depth-3 &&
			ttFlag != UPPER &&
			abs(ttScore) < mate-maxPly {
			sBeta := ttScore - 6*depth
			// The SE sub-search calls search(p, ply, ...) which reinitialises
			// moveBuffers[ply], destroying the main loop's picker state.
			// Save and restore the picker around the sub-search.
			savedPicker := *picker
			excludedMove[ply] = move
			var sePv [maxPly]int
			sScore := search(p, ply, sBeta-1, sBeta, (depth-1)/2, sePv[:])
			excludedMove[ply] = 0
			*picker = savedPicker
			if sScore < sBeta {
				extension = 1
			}
		}

		// Capture piece type before makeMove — after the call the square
		// may hold a promoted piece rather than the original pawn.
		movedPiece := p.typeAt(moveFrom(move))

		var u Undo
		makeMove(p, move, &u)
		if p.selfInCheck() { // move left our king in check: illegal
			unmakeMove(p, move, &u)
			continue
		}

		// Record this move in the cont hist context stack so child nodes
		// can look it up as their "1-ply back" context.
		// p.side has flipped after makeMove, so the mover was p.side^1.
		contSide[ply] = p.side ^ 1
		contPiece[ply] = movedPiece
		contTo[ply] = moveTo(move)
		contValid[ply] = true

		givesCheck := p.inCheck()

		// Extend by one ply for moves that give check, plus singular extension.
		newDepth := depth - 1 + extension
		if givesCheck {
			newDepth++
		}

		// Late move pruning: skip quiet moves beyond the threshold.
		// Moves that give check are exempt — they may be the only defence
		// or the only escape from a mating attack.
		// When improving we allow more moves (position is trending up, so
		// later moves are more likely to be relevant).
		lmpThreshold := 4*depth + 1
		if improving {
			lmpThreshold = 6*depth + 1
		}
		if stage == StageQuiet && !isPv && !nodeInCheck && depth < 4 &&
			quietTried > lmpThreshold && !givesCheck {
			unmakeMove(p, move, &u)
			continue
		}
		if stage == StageQuiet {
			quietTried++
		}

		// Late move reduction
		isReduced := false
		if stage == StageQuiet && depth > 2 && !nodeInCheck && !givesCheck && movesTried >= 4 {
			reduction := lmr[min(depth, 63)][min(movesTried, 63)]
			if reduction > 0 {
				if !isPv {
					reduction++
				}
				// Not improving: position is trending down, reduce more aggressively.
				if !improving {
					reduction++
				}
				if reduction > newDepth-1 {
					reduction = newDepth - 1
				}
				score = -search(p, ply+1, -alpha-1, -alpha, newDepth-reduction, childPv[:])
				if score <= alpha {
					isReduced = true
				}
			}
		}

		// Principal Variation Search:
		// First move: full window.
		// Subsequent moves: zero-width window first; re-search with full window
		// only if the ZW fails high AND we are at a PV node.
		if !isReduced {
			if movesTried == 0 {
				score = -search(p, ply+1, -beta, -alpha, newDepth, childPv[:])
			} else {
				score = -search(p, ply+1, -alpha-1, -alpha, newDepth, childPv[:])
				if score > alpha && isPv {
					score = -search(p, ply+1, -beta, -alpha, newDepth, childPv[:])
				}
			}
		}
		unmakeMove(p, move, &u)
		movesTried++

		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}

		// Beta cutoff: this move is "too good"; the opponent won't allow
		// reaching this position, so we can stop searching.  Apply a malus
		// to every quiet that was searched before this one — they failed to
		// cut off and should be tried later in future sibling nodes.
		if score >= beta {
			if isQuiet(p, move) {
				updateHistory(p, move, depth, ply, quietsMade[:quietsMadeCount])
			}
			if excludedMove[ply] == 0 {
				storeTT(p.key, move, score, LOWER, depth, ply)
			}
			return score
		}

		// Record this quiet as searched-but-failed so we can penalise it
		// if a later move causes a cutoff.
		if stage == StageQuiet && quietsMadeCount < maxMoves {
			quietsMade[quietsMadeCount] = move
			quietsMadeCount++
		}

		if score > bestScore {
			bestScore = score
			if score > alpha {
				alpha = score
				bestMove = move
				buildPV(pv, childPv[:], move)
				if isRoot {
					reportInfo(score, pv)
				}
			}
		}
	}

	// --- Handle terminal nodes ---
	if bestScore == -inf {
		// In a singular extension sub-search the excluded move may have been
		// the only legal move; that's not checkmate or stalemate, just failure.
		if excludedMove[ply] != 0 {
			return alpha
		}
		if p.inCheck() {
			return -mate + ply // checkmate: prefer shorter mates
		}
		return 0 // stalemate
	}

	// 50-move rule: only reached when there are legal moves, so checkmate
	// and stalemate have already been handled above and take priority.
	if !isRoot && p.clock >= 100 {
		return 0
	}

	// Store the result in the TT with the appropriate bound type.
	// Skip during singular extension sub-searches: their partial results
	// (with one move excluded) must not corrupt the main TT entries.
	if excludedMove[ply] == 0 {
		if bestMove != 0 {
			if isQuiet(p, bestMove) {
				updateHistory(p, bestMove, depth, ply, nil)
			}
			storeTT(p.key, bestMove, bestScore, EXACT, depth, ply)
		} else {
			storeTT(p.key, 0, bestScore, UPPER, depth, ply)
		}
	}
	return bestScore
}

// quiesce searches captures (and, when in check, all moves) until the
// position is quiet, then returns the static evaluation.  This prevents
// the "horizon effect" where the engine ignores an imminent capture at
// the leaf.
//
// When the side to move is in check we cannot stand pat — an evasion
// must be found.  We fall through to the full move pipeline so that
// quiet evasions are included.  If no legal move exists we return a
// checkmate score.  Outside of check the standard stand-pat applies.
func quiesce(p *Pos, ply, alpha, beta int, pv []int) int {
	nodes++
	if ply > selDepth {
		selDepth = ply
	}
	checkTime()
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	pv[0] = 0

	// Repetition detection
	if isRepetition(p) {
		return 0
	}
	if p.clock >= 100 || p.isInsufficientMaterial() {
		return 0
	}
	if atomic.LoadInt32(&abortFlag) != 0 {
		return 0
	}

	// Safeguard against reaching max ply limit
	if ply >= maxPly-1 {
		return evaluate(p)
	}

	// TT probe: use depth=0 for all qsearch entries.
	ttMove := 0
	ttScore := 0
	ttFlag := 0
	ttDepth := 0
	if probeTT(p.key, &ttMove, &ttScore, &ttFlag, &ttDepth, alpha, beta, 0, ply) {
		return ttScore
	}

	inCheck := p.inCheck()

	picker := &moveBuffers[ply]
	var childPv [maxPly]int

	// Stand-pat: outside of check we may decline all captures.
	// In check we must find an evasion, so stand-pat is illegal.
	best := -inf
	if !inCheck {
		best = evaluate(p)
		if best >= beta {
			return best
		}
		if best > alpha {
			alpha = best
		}
		initQSearch(p, picker)
	} else {
		initMovePicker(p, picker, 0, ply)
	}

	movesTried := 0
	for {
		var move int
		if inCheck {
			move, _ = picker.nextMove()
			if move == 0 {
				break
			}
		} else {
			move = picker.nextCapture()
			if move == 0 {
				break
			}
			if isBadCapture(p, move) {
				continue
			}
		}

		var u Undo
		makeMove(p, move, &u)
		if p.selfInCheck() {
			unmakeMove(p, move, &u)
			continue
		}
		movesTried++
		score := -quiesce(p, ply+1, -beta, -alpha, childPv[:])
		unmakeMove(p, move, &u)

		if atomic.LoadInt32(&abortFlag) != 0 {
			return 0
		}
		if score >= beta {
			return score
		}
		if score > best {
			best = score
			if score > alpha {
				alpha = score
				buildPV(pv, childPv[:], move)
			}
		}
	}

	// In check with no legal evasion: checkmate.
	if inCheck && movesTried == 0 {
		return -mate + ply
	}

	return best
}

// isRepetition returns true if the current position is a draw by repetition.
//
// Two rules apply depending on where the prior occurrence lives:
//
//	In-tree (keyHist index >= rootHistLen): the position was created during
//	the current search. One prior occurrence is enough to return draw — the
//	opponent can always force the third occurrence on the real board.
//
//	In-history (keyHist index < rootHistLen): the position existed before the
//	search started. Strict threefold requires two prior occurrences (three
//	total) before we can claim a forced draw.
//
// We step back in increments of 2 (same side to move) and stop at the
// 50-move clock boundary, since repetitions cannot cross an irreversible move.
func isRepetition(p *Pos) bool {
	reps := 0
	for i := 4; i <= p.clock; i += 2 {
		idx := p.histLen - i
		if idx < 0 {
			break
		}
		if p.key != p.keyHist[idx] {
			continue
		}
		if idx >= rootHistLen {
			return true // in-tree: one occurrence is enough
		}
		reps++
		if reps >= 2 {
			return true // in-history: need two prior occurrences
		}
	}
	return false
}

// reportInfo outputs a UCI "info" line for the current iteration.
// Mate scores are converted to "moves to mate" format.
func reportInfo(score int, pv []int) {
	// If we are approaching checkmate for either side, calculate
	// distance to checkmate; in the normal scenario, switch to
	// centipawns.
	scoreType := "mate"
	if score < -maxEval {
		score = (-mate - score) / 2
	} else if score > maxEval {
		score = (mate - score + 1) / 2
	} else {
		scoreType = "cp"
	}

	// Set elapsed (time used so far), and guard
	// against division by zero in nps calculation
	elapsed := time.Now().UnixMilli() - searchStart
	if elapsed <= 0 {
		elapsed = 1
	}

	// Calculate nodes per second
	nps := nodes * 1000 / elapsed

	// Output
	hashfull := ttHashfull()
	fmt.Printf("info depth %d seldepth %d time %d nodes %d nps %d hashfull %d score %s %d pv %s\n",
		rootDepth, selDepth, elapsed, nodes, nps, hashfull, scoreType, score, pvString(pv))
}

// checkTime is called periodically (every 4096 nodes) to see whether
// the allocated time has expired.  If so, it sets abortFlag so the
// search unwinds cleanly.  Skipped during depth-1 searches (the
// engine must always return at least one move) and when pondering.
func checkTime() {
	if nodes&1023 != 0 || rootDepth == 1 {
		return
	}
	if !pondering && timeLimit >= 0 {
		if time.Now().UnixMilli()-searchStart >= timeLimit {
			atomic.StoreInt32(&abortFlag, 1)
		}
	}
}
