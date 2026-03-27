// ================================================================
// BITBOARD HELPERS
// ================================================================
//

package main

const (
	excludeA uint64 = 0xfefefefefefefefe
	excludeH uint64 = 0x7f7f7f7f7f7f7f7f
)

// Shift helpers. White moves north, i.e. toward higher ranks.

func ShiftNorth(b uint64) uint64 { return b << 8 }
func ShiftSouth(b uint64) uint64 { return b >> 8 }
func ShiftWest(b uint64) uint64  { return (b & excludeA) >> 1 }
func ShiftEast(b uint64) uint64  { return (b & excludeH) << 1 }
func ShiftNW(b uint64) uint64    { return (b & excludeA) << 7 }
func ShiftNE(b uint64) uint64    { return (b & excludeH) << 9 }
func ShiftSW(b uint64) uint64    { return (b & excludeA) >> 9 }
func ShiftSE(b uint64) uint64    { return (b & excludeH) >> 7 }

// Side-dependent forward shift.
func ShiftForward(b uint64, side int) uint64 {
	if side == White {
		return ShiftNorth(b)
	}
	return ShiftSouth(b)
}

// Shift to both sides.
func ShiftSides(b uint64) uint64 {
	return ShiftWest(b) | ShiftEast(b)
}

// White pawn attacks.
func ShiftWPAttacks(b uint64) uint64 {
	return ShiftNE(b) | ShiftNW(b)
}

// Black pawn attacks.
func ShiftBPAttacks(b uint64) uint64 {
	return ShiftSE(b) | ShiftSW(b)
}