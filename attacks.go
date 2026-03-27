// ================================================================
// S3  ATTACK DETECTION
// ================================================================
//
//   Three functions answer the core question "is this square safe?":
//
//   attacksFrom(p, sq)
//       Returns a bitboard of squares that the piece ON sq can reach.
//       Used in legality checking to confirm a piece can actually
//       move to its alleged destination.
//
//   attacksTo(p, sq)
//       Returns a bitboard of ALL pieces (both sides) that attack sq.
//       Used as the starting point for static exchange evaluation
//       (see swap.go).
//
//   isAttacked(p, sq, side)
//       Boolean: is sq attacked by any piece of the given side?
//       Used for check detection and castling legality.
//
//   All three rely on the precomputed tables in tables.go.  The key
//   insight is that attack detection is symmetric for sliding pieces:
//   a bishop on A1 attacks H8 if and only if the rook on H8 would
//   see A1, so we shoot a ray FROM the target square and intersect
//   with the actual piece bitboards.
//

package main

// attacksFrom returns a bitboard of squares that the piece on sq
// can move to (ignoring legality; the caller filters illegal moves).
// Sliding piece attacks account for the current occupancy.
func attacksFrom(p *Pos, sq int) uint64 {
	occ := p.occupied()
	switch p.typeAt(sq) {
	case P:
		return pawnAtk[colorOf(p.board[sq])][sq]
	case N:
		return knightAtk[sq]
	case B:
		return bishopAttacks(occ, sq)
	case R:
		return rookAttacks(occ, sq)
	case Q:
		return queenAttacks(occ, sq)
	case K:
		return kingAtk[sq]
	}
	return 0
}

// attacksTo returns a bitboard of every piece (of either color) that
// attacks the given square.  This is the "sonar ping" function:
// shoot rays from sq in all directions and see what you hit.
func attacksTo(p *Pos, sq int) uint64 {
	occ := p.occupied()
	return (p.pieceBB(White, P) & pawnAtk[Black][sq]) | // White pawns attack from below
		(p.pieceBB(Black, P) & pawnAtk[White][sq]) |    // Black pawns attack from above
		(p.typeBB[N] & knightAtk[sq]) |
		((p.typeBB[B] | p.typeBB[Q]) & bishopAttacks(occ, sq)) |
		((p.typeBB[R] | p.typeBB[Q]) & rookAttacks(occ, sq)) |
		(p.typeBB[K] & kingAtk[sq])
}

// isAttacked reports whether sq is attacked by any piece of side.
// This is a short-circuit version of attacksTo: it returns as soon
// as the first attacker is found, making it faster for check tests.
func isAttacked(p *Pos, sq, side int) bool {
	occ := p.occupied()
	return (p.pieceBB(side, P)&pawnAtk[opp(side)][sq] != 0) ||
		(p.pieceBB(side, N)&knightAtk[sq] != 0) ||
		((p.pieceBB(side, B)|p.pieceBB(side, Q))&bishopAttacks(occ, sq) != 0) ||
		((p.pieceBB(side, R)|p.pieceBB(side, Q))&rookAttacks(occ, sq) != 0) ||
		(p.pieceBB(side, K)&kingAtk[sq] != 0)
}
