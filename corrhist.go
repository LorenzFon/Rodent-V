package main

/*
Corrhist is a shorthand for *Static Evaluation Correction History*
- an algorithm that adjusts static eval based on how it differs
from search result. Average difference is accumulated in the tables
indexed by hash keys related to certain aspects of position (pawn
placement, non-pawn placement etc.) and added to static eval.
The big idea is that such correction captures things missed by
evaluation function.

Good external descriptions of the algorithm can be found at
https://int0x80.ca/posts/chess-engines/19-corrhist or
https://www.chessprogramming.org/Static_Evaluation_Correction_History

Rodent uses:
- pawn corection history
- non-pawn correction history
*/

// Correction history constants.
const (
	corrHistSize        = 16384              // number of entries per side
	corrHistGrain       = 512                // scaling factor for diff
	corrHistWeightScale = 256                // weight divisor
	corrHistMax         = corrHistGrain * 32 // absolute clamp
)

// Corrhist tables are per-thread fields inside SearchState (thread.go).

// getCorrection returns the total correction value for the position,
// combining pawn and non-pawn correction history.
func (ss *SearchState) getCorrection(p *Pos) int {
	side := p.side
	pawnIdx := int((p.pawnKey[White] ^ p.pawnKey[Black]) % corrHistSize)
	corr := int(ss.pawnCorrHist[side][pawnIdx] / corrHistGrain)
	corr += int(ss.nonPawnCorrHist[White][side][int(p.nonPawnKey[White]%corrHistSize)]) / corrHistGrain
	corr += int(ss.nonPawnCorrHist[Black][side][int(p.nonPawnKey[Black]%corrHistSize)]) / corrHistGrain
	return corr
}

// updateCorrEntry applies a weighted update to a single correction history entry.
func updateCorrEntry(entry *int16, newWeight, scaledDiff int) {
	old := int(*entry)
	update := old*(corrHistWeightScale-newWeight) + scaledDiff*newWeight
	update /= corrHistWeightScale
	update = max(-corrHistMax, min(corrHistMax, update))
	*entry = int16(update)
}

// addCorrection updates all correction history entries for the position.
func (ss *SearchState) addCorrection(p *Pos, depth, diff int) {
	side := p.side
	newWeight := min(16, 1+depth)
	scaledDiff := diff * corrHistGrain
	pawnIdx := int((p.pawnKey[White] ^ p.pawnKey[Black]) % corrHistSize)
	updateCorrEntry(&ss.pawnCorrHist[side][pawnIdx], newWeight, scaledDiff)
	updateCorrEntry(&ss.nonPawnCorrHist[White][side][int(p.nonPawnKey[White]%corrHistSize)], newWeight, scaledDiff)
	updateCorrEntry(&ss.nonPawnCorrHist[Black][side][int(p.nonPawnKey[Black]%corrHistSize)], newWeight, scaledDiff)
}
