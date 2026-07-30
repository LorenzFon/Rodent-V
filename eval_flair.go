// ================================================================
// EVAL FLAIRS
// ================================================================
//
// Previous Rodents had evaluation functions that mixed two aspects
// of eval: things done for strength and things done for aeshetics.
// Rodent V tries to reduce complexity by moving the aestethics part
// to separate small functions.
//
// flairClosed() - preference for closed positions
// flairTropism() - moving pieces towards enemy king

package main

// tropism values come from GambitFruit,
// https://github.com/Randl/GambitFruit/blob/master/GambitFruit/eval.cpp#L80
var tropismMG = [6]int{P: 0, N: 3, B: 2, R: 2, Q: 2, K: 0}
var tropismEG = [6]int{P: 0, N: 3, B: 1, R: 1, Q: 4, K: 0}

func flairClosed(p *Pos) int {
	var score [2]int
	score[White] = 0
	score[Black] = 0

	for side := White; side <= Black; side++ {

		// Increase knight value with more pawns on the board
		score[side] += 6 * p.count[side][N] * (p.count[side][P] - 4)

		// Upgrade own pawn value, downgrade bishop and rook values
		score[side] += 2 * p.count[side][P] // max +16
		score[side] -= 6 * p.count[side][B] // max -12
		score[side] -= 2 * p.count[side][R] // max -4
		//                                  // total: 0
	}

	var result = score[White] - score[Black]

	// Return score from the perspective of the side to move.
	if p.side == White {
		return result
	}
	return -result
}

func flairTropism(p *Pos) int {
	var mg [2]int
	var eg [2]int
	var phase int = 0
	mg[White] = 0
	mg[Black] = 0
	eg[White] = 0
	eg[Black] = 0

	for side := White; side <= Black; side++ {
		for pt := N; pt <= Q; pt++ {
			pieces := p.pieceBB(side, pt)

			for pieces != 0 {
				sq := lsb(pieces)
				pieces &= pieces - 1
				dst := distBonus(sq, p.kingSq[side^1])

				mg[side] += tropismMG[pt] * dst
				eg[side] += tropismEG[pt] * dst
				phase += gamePhase[pt] // gamePhase borrowed from pesto.go
			}
		}
	}

	// Clamp phase.
	if phase > 24 {
		phase = 24
	}

	// Interpolate between midgame/endgame scores.
	mgScore := mg[White] - mg[Black]
	egScore := eg[White] - eg[Black]
	var result = (mgScore*phase + egScore*(24-phase)) / 24

	// Return score from the perspective of the side to move.
	if p.side == White {
		return result
	}
	return -result

}

func distBonus(s1, s2 int) int {
	return 7 - chebyshev(s1, s2)
}
