// ================================================================
// S4  MAKE / UNMAKE MOVE
// ================================================================
//
//   makeMove() and unmakeMove() are the engine's most critical
//   functions.  Every field in Pos that is kept incrementally
//   (bitboards, piece array, material, PST score, Zobrist key,
//   castling rights, en-passant square) must be updated here.
//
//   DESIGN PRINCIPLE
//   ----------------
//   We do NOT copy the full position before each move.  Instead,
//   makeMove() saves just the fields that cannot be reconstructed
//   from the move alone into an Undo struct, then unmakeMove()
//   restores them.  This is the "incremental update" style, and it
//   is faster than full copying because most fields (bitboards, PST
//   scores, material) can be recomputed by reversing the same
//   arithmetic.
//
//   MOVE TYPES
//   ----------
//   NORMAL: move the piece, remove a captured piece (if any).
//   CASTLE: move king AND rook atomically; the rook lands on the
//           square between king's old and new position.
//   EP_CAP: the captured pawn is NOT on "to" but one square
//           behind it (to ^ 8 toggles rank by 1).
//   EP_SET: double pawn push; set epSquare for next ply.
//   ?_PROM: the pawn on "from" becomes a new piece on "to".
//
//   NULL MOVE
//   ---------
//   makeNullMove() passes the turn without moving any piece.  It is
//   used in null-move pruning (see search.go).  The Zobrist key still
//   gets the sideKey XOR; the en-passant key is cleared if needed.
//

package main

// makeMove applies move to position p.  The Undo record u must be
// passed in so that unmakeMove() can restore the position exactly.
func makeMove(p *Pos, move int, u *Undo) {
	side := p.side
	from := moveFrom(move)
	to := moveTo(move)
	fromType := p.typeAt(from)
	toType := p.typeAt(to)

	// Save fields that will be destroyed by this move.
	u.captured = toType
	u.castleRights = p.castleRights
	u.epSquare = p.epSquare
	u.clock = p.clock
	u.key = p.key
	u.pawnKey = p.pawnKey
	u.nonPawnKey = p.nonPawnKey

	// Append current key to the repetition history.
	p.keyHist[p.histLen] = p.key
	p.histLen++

	// 50-move clock: resets on pawn moves and captures.
	if fromType == P || toType != NO_TP {
		p.clock = 0
	} else {
		p.clock++
	}

	// Update castling rights: strip any rights affected by a piece
	// moving from or being captured on a corner/king square.
	p.key ^= zobCastle[p.castleRights]
	p.castleRights &= castleMask[from] & castleMask[to]
	p.key ^= zobCastle[p.castleRights]

	// Clear old en-passant key contribution.
	if p.epSquare != NO_SQ {
		p.key ^= zobEP[fileOf(p.epSquare)]
		p.epSquare = NO_SQ
	}

	// --- Move the piece from -> to ---
	p.board[from] = NO_PC
	p.board[to] = makePiece(side, fromType)
	p.key ^= zobPiece[makePiece(side, fromType)][from] ^
		zobPiece[makePiece(side, fromType)][to]
	if fromType == P {
		p.pawnKey[side] = p.pawnKey[side] ^ zobPiece[makePiece(side, fromType)][from] ^
			zobPiece[makePiece(side, fromType)][to]
	} else {
		p.nonPawnKey[side] ^= zobPiece[makePiece(side, fromType)][from] ^
			zobPiece[makePiece(side, fromType)][to]
	}
	p.colorBB[side] ^= squareBit(from) | squareBit(to)
	p.typeBB[fromType] ^= squareBit(from) | squareBit(to)

	// --- Update king square ---
	if fromType == K {
		p.kingSq[side] = to
	}

	// --- Handle a normal capture at "to" ---
	if toType != NO_TP {
		p.key ^= zobPiece[makePiece(opp(side), toType)][to]
		if toType == P {
			p.pawnKey[opp(side)] = p.pawnKey[opp(side)] ^ zobPiece[makePiece(opp(side), toType)][to]
		} else if toType != K {
			p.nonPawnKey[opp(side)] ^= zobPiece[makePiece(opp(side), toType)][to]
		}
		p.colorBB[opp(side)] ^= squareBit(to)
		p.typeBB[toType] ^= squareBit(to)
		p.count[opp(side)][toType]--
	}

	// --- Special move type handling ---
	switch moveType(move) {
	case NORMAL:
		// Nothing extra to do.

	case CASTLE:
		// Move the rook atomically with the king.
		// King side: rook goes from +3 to +(-1) relative to king.
		// Queen side: rook goes from -4 to +(+1) relative to king.
		var rookFrom, rookTo int
		if to > from {
			rookFrom = from + 3 // H-file rook
			rookTo = to - 1     // F-file landing
		} else {
			rookFrom = from - 4 // A-file rook
			rookTo = to + 1     // D-file landing
		}
		p.board[rookFrom] = NO_PC
		p.board[rookTo] = makePiece(side, R)
		p.key ^= zobPiece[makePiece(side, R)][rookFrom] ^
			zobPiece[makePiece(side, R)][rookTo]
		p.nonPawnKey[side] ^= zobPiece[makePiece(side, R)][rookFrom] ^
			zobPiece[makePiece(side, R)][rookTo]
		p.colorBB[side] ^= squareBit(rookFrom) | squareBit(rookTo)
		p.typeBB[R] ^= squareBit(rookFrom) | squareBit(rookTo)

	case EP_CAP:
		// The captured pawn sits one square behind "to" (XOR 8 flips rank).
		capSq := to ^ 8
		p.board[capSq] = NO_PC
		p.key ^= zobPiece[makePiece(opp(side), P)][capSq]
		p.pawnKey[opp(side)] = p.pawnKey[opp(side)] ^ zobPiece[makePiece(opp(side), P)][capSq]
		p.colorBB[opp(side)] ^= squareBit(capSq)
		p.typeBB[P] ^= squareBit(capSq)
		p.count[opp(side)][P]--

	case EP_SET:
		// Double pawn push: record the en-passant square if an enemy
		// pawn can actually capture there next move.
		epSq := to ^ 8
		if pawnAtk[side][epSq]&p.pieceBB(opp(side), P) != 0 {
			p.epSquare = epSq
			p.key ^= zobEP[fileOf(epSq)]
		}

	case N_PROM, B_PROM, R_PROM, Q_PROM:
		// Swap the pawn for the promoted piece.
		fromType = promType(move)
		p.board[to] = makePiece(side, fromType)
		p.key ^= zobPiece[makePiece(side, P)][to] ^
			zobPiece[makePiece(side, fromType)][to]
		p.pawnKey[side] = p.pawnKey[side] ^ zobPiece[makePiece(side, P)][to]
		p.nonPawnKey[side] ^= zobPiece[makePiece(side, fromType)][to]
		p.typeBB[P] ^= squareBit(to)
		p.typeBB[fromType] ^= squareBit(to)
		p.count[side][fromType]++
		p.count[side][P]--
	}

	p.side ^= 1
	p.key ^= sideKey
}

// unmakeMove restores position p to the state before move was made.
// u must be the Undo record filled by the corresponding makeMove call.
func unmakeMove(p *Pos, move int, u *Undo) {
	side := opp(p.side) // the side that made the move
	from := moveFrom(move)
	to := moveTo(move)
	movingType := p.typeAt(to) // type of piece now on "to"
	capType := u.captured

	// Restore the saved fields.
	p.castleRights = u.castleRights
	p.epSquare = u.epSquare
	p.clock = u.clock
	p.key = u.key
	p.pawnKey = u.pawnKey
	p.nonPawnKey = u.nonPawnKey
	p.histLen--

	// --- Move piece back: to -> from ---
	p.board[from] = makePiece(side, movingType)
	p.board[to] = NO_PC
	p.colorBB[side] ^= squareBit(from) | squareBit(to)
	p.typeBB[movingType] ^= squareBit(from) | squareBit(to)

	// --- Update king square ---
	if movingType == K {
		p.kingSq[side] = from
	}

	// --- Restore a captured piece ---
	if capType != NO_TP {
		p.board[to] = makePiece(opp(side), capType)
		p.colorBB[opp(side)] ^= squareBit(to)
		p.typeBB[capType] ^= squareBit(to)
		p.count[opp(side)][capType]++
	}

	// --- Reverse special move effects ---
	switch moveType(move) {
	case NORMAL:
		// Nothing extra.

	case CASTLE:
		var rookFrom, rookTo int
		if to > from {
			rookFrom = from + 3
			rookTo = to - 1
		} else {
			rookFrom = from - 4
			rookTo = to + 1
		}
		p.board[rookTo] = NO_PC
		p.board[rookFrom] = makePiece(side, R)
		p.colorBB[side] ^= squareBit(rookFrom) | squareBit(rookTo)
		p.typeBB[R] ^= squareBit(rookFrom) | squareBit(rookTo)

	case EP_CAP:
		capSq := to ^ 8
		p.board[capSq] = makePiece(opp(side), P)
		p.colorBB[opp(side)] ^= squareBit(capSq)
		p.typeBB[P] ^= squareBit(capSq)
		p.count[opp(side)][P]++

	case EP_SET:
		// Nothing extra. epSquare was restored from u above.

	case N_PROM, B_PROM, R_PROM, Q_PROM:
		// Restore the pawn: the piece at "from" is now the promoted
		// piece type, so swap typeBB back.
		p.board[from] = makePiece(side, P)
		p.typeBB[P] ^= squareBit(from)
		p.typeBB[movingType] ^= squareBit(from)
		p.count[side][P]++
		p.count[side][movingType]--
	}

	p.side ^= 1
}

// ================================================================
// NULL MOVE
// ================================================================
//
//   A null move simply flips the side to move without touching any
//   piece.  It is used by null-move pruning: if the position is so
//   good that even "doing nothing" causes a beta cutoff, we can
//   prune the branch.
//
//   Only the Zobrist key and en-passant state need updating; the
//   rest of the position is unchanged.
//

// makeNullMove passes the turn without moving.  Only u.epSquare and
// u.key are saved; the other fields are not touched.
func makeNullMove(p *Pos, u *Undo) {
	u.epSquare = p.epSquare
	u.key = p.key

	p.keyHist[p.histLen] = p.key
	p.histLen++
	p.clock++ // null moves advance the 50-move clock

	if p.epSquare != NO_SQ {
		p.key ^= zobEP[fileOf(p.epSquare)]
		p.epSquare = NO_SQ
	}

	p.side ^= 1
	p.key ^= sideKey
}

// unmakeNullMove reverses makeNullMove.
func unmakeNullMove(p *Pos, u *Undo) {
	p.epSquare = u.epSquare
	p.key = u.key
	p.histLen--
	p.clock--
	p.side ^= 1
}
