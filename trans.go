// ================================================================
// S9  TRANSPOSITION TABLE
// ================================================================
//
//   The transposition table (TT) is a hash map from position keys to
//   search results.  It is the single most important optimisation in
//   an alpha-beta engine: when the same position is reached via
//   different move orders, we reuse the earlier result instead of
//   re-searching.
//
//   STRUCTURE
//   ---------
//   The table is a flat array of Entry values.  Each position maps to
//   a "bucket" of 4 consecutive entries (4-way associative).  On a
//   lookup we check all 4; on a store we replace the one that is
//   oldest or shallowest.
//
//   4-WAY ASSOCIATIVITY
//   -------------------
//   A direct-mapped table (one slot per key) suffers from high
//   collision rates.  Four slots per bucket reduces collisions by
//   almost 4x at a cost of 4x the memory bandwidth, a good trade.
//   ttMask = ttSize - 4 ensures the base index is always aligned to
//   a 4-entry boundary.
//
//   ENTRY FIELDS
//   ------------
//   key: full 64-bit Zobrist key for collision detection.
//   move: best move found (or 0); used as a hint by the next search.
//   score: the search score. Mate scores are stored relative to
//          the root (not relative to the position), so they are
//          adjusted by ply on store and probe.
//   depth: remaining depth when the entry was stored.
//   bound: UPPER, LOWER, or EXACT (see constants in tables.go).
//   date: the search "generation" counter when this was stored;
//         used to prefer recent entries over stale ones.
//
//   REPLACEMENT POLICY
//   ------------------
//   For each bucket we compute an "age score": ((generation - date) * 256)
//   + (255 - depth).  We replace the entry with the highest age score,
//   i.e., the one that is most stale AND shallowest.  A key match
//   always replaces the matched entry to keep data coherent.
//
//   MATE SCORE ADJUSTMENT
//   ----------------------
//   Mate scores in alpha-beta are ply-relative: a mate in 3 from the
//   root has a different raw score if reached at ply 2 vs ply 0.
//   To make stored scores portable across transpositions we adjust:
//       store: score += ply  (if mate-like, i.e. > maxEval)
//       probe: score -= ply
//   This converts between "distance to mate from root" and "distance
//   to mate from this position."
//

package main

// Entry is one slot in the transposition table.  Its size is exactly
// 16 bytes, which keeps entries aligned in the cache.
type Entry struct {
	key   uint64 // full position key for collision detection
	date  int16  // search generation when this was stored (for aging)
	move  int16  // best move (0 if unknown)
	score int16  // search score (mate-adjusted)
	bound uint8  // UPPER, LOWER, or EXACT
	depth uint8  // remaining depth at the time of storage
}

// Global TT state.
var (
	tt     []Entry // the flat entry array
	ttSize int     // total number of entries (power of 2)
	ttMask int     // index mask = ttSize - 4 (aligned buckets)
	ttDate int     // current search generation (0-255)
)

// allocTT allocates a transposition table of approximately mbSize
// megabytes.  The size is rounded down to the nearest power of 2
// so that the index mask trick works.  Always clears the table.
func allocTT(mbSize int) {
	// Find the largest power of 2 that fits within mbSize MB.
	size := 2
	for size <= mbSize {
		size *= 2
	}
	// Each entry is 16 bytes; we want (size/2) MiB of entries.
	ttSize = ((size / 2) << 20) / 16
	ttMask = ttSize - 4
	tt = make([]Entry, ttSize)
	clearTT()
}

// clearTT zeroes the table and resets the date counter.
// Called at the start of a new game (ucinewgame).
func clearTT() {
	ttDate = 0
	for i := range tt {
		tt[i] = Entry{}
	}
}

// ttHashfull returns TT utilization in UCI hashfull units (permille).
// 1000 means table is fully occupied by recent searches.
func ttHashfull() int {
	if ttSize <= 0 {
		return 0
	}

	// Sample the first 1000 entries (or ttSize if small)
	sampleSize := min(ttSize, 1000)

	active := 0
	for i := 0; i < sampleSize; i++ {
		e := &tt[i]
		if e.key == 0 {
			continue
		}
		// Consider an entry active if it's from the current or previous generation.
		// Handling 8-bit wrap around using bitwise AND just like the replacement policy.
		age := (ttDate - int(e.date)) & 255
		if age <= 1 {
			active++
		}
	}

	return (active * 1000) / sampleSize
}

// probeTT looks up a position in the transposition table.
//
// If a matching entry is found at sufficient depth, *move is set to
// the stored best move and *score is set to the stored score.
// Returns true (and sets *score) only when the score can be used
// directly as a cutoff, according to the bound type and alpha/beta.
//
// Even when the score cannot be used for an immediate cutoff, the
// move hint is still returned so the search can try it first.
func probeTT(key uint64, move *int, score *int, flag *int, ttDepth *int, alpha, beta, depth, ply int) bool {
	base := int(key) & ttMask
	bucket := tt[base : base+4]
	for i := range bucket {
		e := &bucket[i]
		if e.key != key {
			continue
		}
		// Refresh the date so this entry is not considered stale.
		e.date = int16(ttDate)
		*move = int(e.move)
		*flag = int(e.bound)
		*ttDepth = int(e.depth)

		// Always decode the score on a key match so callers (e.g. singular
		// extensions) can use it even when depth is insufficient for a cutoff.
		*score = int(e.score)
		if *score < -maxEval {
			*score += ply
		} else if *score > maxEval {
			*score -= ply
		}

		if int(e.depth) >= depth {
			// Use the score if it can produce a cutoff:
			//   EXACT: always usable.
			//   LOWER: score is a lower bound; usable when it already beats beta.
			//   UPPER: score is an upper bound; usable when it already fails alpha.
			if int(e.bound) == EXACT ||
				(int(e.bound)&UPPER != 0 && *score <= alpha) ||
				(int(e.bound)&LOWER != 0 && *score >= beta) {
				return true
			}
		}
		break // key matched but depth or bound insufficient
	}
	return false
}

// storeTT writes a search result to the transposition table.
// If the position's key already occupies a slot in the bucket, that
// slot is reused (preserving the move hint if the new search has
// none).  Otherwise, the oldest/shallowest entry is evicted.
func storeTT(key uint64, move, score, bound, depth, ply int) {
	// Adjust mate scores to be position-relative (not ply-relative).
	if score < -maxEval {
		score -= ply
	} else if score > maxEval {
		score += ply
	}

	base := int(key) & ttMask
	bucket := tt[base : base+4]
	var replace *Entry
	oldest := -1

	for i := range bucket {
		e := &bucket[i]
		if e.key == key {
			// Reuse the existing slot; preserve the move if we have none.
			if move == 0 {
				move = int(e.move)
			}
			replace = e
			break
		}
		// Age score: prefer to replace stale, shallow entries.
		age := ((ttDate-int(e.date))&255)*256 + (255 - int(e.depth))
		if age > oldest {
			oldest = age
			replace = e
		}
	}
	if replace == nil {
		replace = &bucket[0]
	}

	replace.key = key
	replace.date = int16(ttDate)
	replace.move = int16(move)
	replace.score = int16(score)
	replace.bound = uint8(bound)
	replace.depth = uint8(depth)
}
