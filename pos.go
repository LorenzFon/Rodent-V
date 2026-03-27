// ================================================================
// S2  BOARD REPRESENTATION
// ================================================================
//
//   Sungorus uses a hybrid representation:
//
//   1. BITBOARDS (colorBB, typeBB)
//      A uint64 per color and per piece-type.  Any intersection
//      gives the set of squares occupied by a specific piece:
//
//          pieceBB(White, R)  =  colorBB[White] & typeBB[R]
//
//      Bitboards make bulk operations (generating all rook moves,
//      counting pawns on a file) cheap and branch-free.
//
//   2. PIECE ARRAY (board[64])
//      One int per square encoding the piece (or NO_PC).  Needed
//      whenever we want "what piece is on sq?" without a loop
//      through bitboards.  make / unmake keep both in sync.
//
//   INCREMENTAL SCORES
//   ------------------
//   material[color] and pstScore[color] are updated as pieces move
//   or are captured so that evaluate() can compute the score in O(1)
//   rather than re-scanning all 64 squares each time.
//
//   ZOBRIST HASH KEY
//   ----------------
//   key is the Zobrist fingerprint of the current position.  It is
//   updated incrementally in makeMove / makeNullMove so that the
//   transposition table lookup is a single array index, not a scan.
//
//   REPETITION DETECTION
//   --------------------
//   keyHist stores the Zobrist key at every position since the last
//   pawn move or capture (the 50-move clock resets histLen at those
//   points).  isRepetition() walks backward in steps of 2 (only
//   same-side positions can repeat) and checks for a key match.
//

package main

import "fmt"

// Pos holds the complete, self-consistent state of a chess position.
// Every field can be derived from the board array alone, but the
// redundant fields are maintained incrementally for speed.
type Pos struct {
	colorBB     [2]uint64  // colorBB[c]: bitboard of all pieces of color c
	typeBB      [6]uint64  // typeBB[t]: bitboard of all pieces of type t
	board       [64]int    // board[sq]: piece on that square, or NO_PC
	kingSq      [2]int     // kingSq[c]: square of the king of color c
	material    [2]int     // material[c]: sum of pieceValue for all pieces of color c
	side        int        // side to move: White or Black
	castleRights int       // castling availability: bit0=WK, bit1=WQ, bit2=BK, bit3=BQ
	epSquare    int        // en-passant target square, or NO_SQ
	clock       int        // half-move clock for the 50-move rule
	histLen     int        // number of keys stored in keyHist
	key         uint64     // Zobrist hash of the current position
	keyHist     [256]uint64 // hash keys since last irreversible move
}

// Undo stores the information needed to reverse a single move.
// The fields that cannot be recovered from the board alone (captured
// piece type, castling flags, en-passant square, 50-move clock) are
// saved here before makeMove() modifies them.
type Undo struct {
	captured    int    // type of the piece that was captured (NO_TP if none)
	castleRights int   // castleRights before the move
	epSquare    int    // epSquare before the move
	clock       int    // half-move clock before the move
	key         uint64 // Zobrist key before the move
}

// ---- Position query helpers ----

// occupied returns a bitboard of all occupied squares.
func (p *Pos) occupied() uint64 { return p.colorBB[White] | p.colorBB[Black] }

// empty returns a bitboard of all empty squares.
func (p *Pos) empty() uint64 { return ^p.occupied() }

// pieceBB returns the bitboard of pieces of the given color and type.
func (p *Pos) pieceBB(color, pieceType int) uint64 {
	return p.colorBB[color] & p.typeBB[pieceType]
}

// typeAt returns the piece type on sq (NO_TP if the square is empty).
func (p *Pos) typeAt(sq int) int { return typeOf(p.board[sq]) }

// inCheck reports whether the side to move is currently in check.
func (p *Pos) inCheck() bool {
	return isAttacked(p, p.kingSq[p.side], opp(p.side))
}

// selfInCheck reports whether the side that just moved has left its
// own king in check (i.e. the move was illegal). Called after
// makeMove() before committing to the position.
func (p *Pos) selfInCheck() bool {
	return isAttacked(p, p.kingSq[opp(p.side)], p.side)
}

// canNullMove reports whether a null move (passing the turn) is safe
// to try.  We skip null moves when only pawns and kings remain on the
// moving side, because the zugzwang risk would make the pruning
// unsound in practice.
func (p *Pos) canNullMove() bool {
	return p.colorBB[p.side]&^(p.typeBB[P]|p.typeBB[K]) != 0
}

// ================================================================
// FEN / EPD PARSER
// ================================================================
//
//   parseFEN reads a position string in EPD (or FEN) format:
//
//       "<piece-placement> <side> <castling> <ep-square>"
//
//   Piece placement: ranks 8 down to 1, "/" between ranks, digits
//   for empty runs, letters for pieces (uppercase = White).
//
//   En-passant square: only set if an enemy pawn can actually
//   capture there (avoids phantom EP entries in the hash).
//

func parseFEN(p *Pos, epd string) {
	const pieceChars = "PpNnBbRrQqKk"

	// Clear all incremental scores.
	for c := 0; c < 2; c++ {
		p.colorBB[c] = 0
		p.material[c] = 0
	}
	for t := 0; t < 6; t++ {
		p.typeBB[t] = 0
	}
	p.castleRights = 0
	p.clock = 0
	p.histLen = 0

	idx := 0 // cursor into epd string

	// --- Piece placement (rank 8 down to rank 1) ---
	for rank := 56; rank >= 0; rank -= 8 {
		file := 0
		for file < 8 {
			ch := epd[idx]
			if ch >= '1' && ch <= '8' {
				// Empty run: fill with NO_PC.
				cnt := int(ch - '0')
				for n := 0; n < cnt; n++ {
					p.board[rank+file] = NO_PC
					file++
				}
			} else {
				// Find the piece in the lookup string.
				pc := 0
				for pc < 12 && pieceChars[pc] != ch {
					pc++
				}
				sq := rank + file
				p.board[sq] = pc
				p.colorBB[colorOf(pc)] ^= squareBit(sq)
				p.typeBB[typeOf(pc)] ^= squareBit(sq)
				if typeOf(pc) == K {
					p.kingSq[colorOf(pc)] = sq
				}
				p.material[colorOf(pc)] += pieceValue[typeOf(pc)]
				file++
			}
			idx++
		}
		idx++ // skip '/' or trailing space
	}

	// --- Side to move ---
	if epd[idx] == 'w' {
		p.side = White
	} else {
		p.side = Black
	}
	idx += 2 // advance past side char and space

	// --- Castling rights ---
	if epd[idx] == '-' {
		idx++
	} else {
		if idx < len(epd) && epd[idx] == 'K' { p.castleRights |= 1; idx++ }
		if idx < len(epd) && epd[idx] == 'Q' { p.castleRights |= 2; idx++ }
		if idx < len(epd) && epd[idx] == 'k' { p.castleRights |= 4; idx++ }
		if idx < len(epd) && epd[idx] == 'q' { p.castleRights |= 8; idx++ }
	}
	idx++ // skip space

	// --- En passant square ---
	// Only record it if a pawn of the side to move can actually
	// execute the capture, otherwise the hash entry would be wrong.
	if idx >= len(epd) || epd[idx] == '-' {
		p.epSquare = NO_SQ
	} else {
		epFile := int(epd[idx] - 'a')
		epRank := int(epd[idx+1] - '1')
		sq := makeSquare(epFile, epRank)
		if pawnAtk[opp(p.side)][sq]&p.pieceBB(p.side, P) != 0 {
			p.epSquare = sq
		} else {
			p.epSquare = NO_SQ
		}
	}

	p.key = computeZobrist(p)
}

// ================================================================
// ZOBRIST KEY FROM SCRATCH
// ================================================================
//
//   computeZobrist rebuilds the position's Zobrist hash by scanning
//   all 64 squares.  This is called only once per parseFEN(); after
//   that the key is kept up to date incrementally in makeMove().
//

func computeZobrist(p *Pos) uint64 {
	var key uint64
	for sq := 0; sq < 64; sq++ {
		if p.board[sq] != NO_PC {
			key ^= zobPiece[p.board[sq]][sq]
		}
	}
	key ^= zobCastle[p.castleRights]
	if p.epSquare != NO_SQ {
		key ^= zobEP[fileOf(p.epSquare)]
	}
	if p.side == Black {
		key ^= sideKey
	}
	return key
}

func PrintBoard(p *Pos) {
	pieceName := []string{
		"P ", "p ",
		"N ", "n ",
		"B ", "b ",
		"R ", "r ",
		"Q ", "q ",
		"K ", "k ",
		". ",
	}

	fmt.Println("--------------------------------------------")
	fmt.Print("  ")

	for sq := 0; sq < 64; sq++ {
		mappedSq := sq ^ 56
		fmt.Print(pieceName[p.board[mappedSq]])

		if (sq+1)%8 == 0 {
			fmt.Printf("  %d\n", 8-sq/8)
			if sq != 63 {
				fmt.Print("  ")
			}
		}
	}

	fmt.Println()
	fmt.Println("  a b c d e f g h")
	fmt.Println()
	fmt.Println("--------------------------------------------")
}