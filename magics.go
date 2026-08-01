// ================================================================
// MAGIC BITBOARD ATTACK TABLES
// ================================================================
//
//   index = ((occ & mask) * magic) >> shift
//
//   Each square has an offset into a flat attack array.  The lookup
//   is a single array access with no pointer indirection:
//
//       flat[m.offset + int((occ & m.mask) * m.magic >> m.shift)]
//
//   Flat table sizes (exact, no padding):
//     Bishop: sum of 2^popcount(bishopMask(sq)) =   5,248 entries ( 41 KiB)
//     Rook:   sum of 2^popcount(rookMask(sq))   = 102,400 entries (800 KiB)
//
//   magicEntry is 32 bytes (mask, magic, shift, offset) so two
//   entries fit in one 64-byte cache line.
//
//   To regenerate the magic numbers run:
//     .\rodent_v.exe genmagics
//

package main

import (
	"fmt"
	"math/rand"
)

// magicEntry is the Fancy Magic descriptor for one piece/square.
// 32 bytes total: two entries fit per 64-byte cache line.
type magicEntry struct {
	mask   uint64 // relevant occupancy bits (board edges excluded)
	magic  uint64 // magic multiplier
	shift  uint   // 64 - popcount(mask)
	offset int    // index into bishopFlat or rookFlat
}

// Flat attack tables - exact sizes, no per-square padding.
const (
	bishopFlatSize = 5248   // sum of 2^popcount(bishopMask(sq)) for all squares
	rookFlatSize   = 102400 // sum of 2^popcount(rookMask(sq)) for all squares
)

var (
	bishopFlat   [bishopFlatSize]uint64
	rookFlat     [rookFlatSize]uint64
	bishopMagics [64]magicEntry
	rookMagics   [64]magicEntry
)

// Precomputed magic numbers - regenerate with: .\rodent_v.exe genmagics
var magicsBishop = [64]uint64{
	0x00200d900e088460, 0x01482a9082060042, 0x4204080085003000, 0x0014040080020000,
	0x000404a000808800, 0x210a080208024080, 0x0001040504400880, 0x0101008200a00490,
	0x00000c100e480108, 0x1000040800b10208, 0x090010041040408c, 0x0800082044400060,
	0x0100020210480000, 0x2001072808401030, 0x080c254422084018, 0x0803008084300608,
	0x001020e08202c800, 0x0021100411220200, 0x0110010a00220863, 0x1208002282004000,
	0x0006138401200000, 0x2001008090049002, 0x2049b08c42101000, 0x40010000804d1004,
	0x300820102a121008, 0x908804900e100200, 0x9004020190008010, 0x4002048018008100,
	0x2001840000802002, 0x008202000048022c, 0x8c0084000a070400, 0x0029010104208800,
	0x0602104400c00808, 0x20280430040c0101, 0x0800440242100100, 0x0428020080080080,
	0x1410068200002200, 0x000a020200804802, 0x002242040a060980, 0x21050400a1010b00,
	0x80080807500008a0, 0x0204110308201100, 0x0520108401209000, 0x00c0104010462600,
	0x2040884500408400, 0x0034100542000040, 0x0002100403280080, 0x0010188100442100,
	0x0000820721200008, 0x40204208047600a2, 0x0000030041102c12, 0x2080820184041000,
	0x0092041006061000, 0x00800510a21200c0, 0x8840380801104002, 0x0094184204106020,
	0x201100c210130880, 0x0420026092282088, 0x99a1002100884400, 0x08080c4442420602,
	0x4001a6000450440c, 0x8200a82420040309, 0x0079110210240090, 0x3004900448002042,
}

var magicsRook = [64]uint64{
	0x0980002180400851, 0x0880304001816000, 0x2500104100200408, 0x8180248008003000,
	0x6200020088102044, 0x0100086400020100, 0x0080120000800100, 0x0200004401188022,
	0x914180002190400c, 0xc051804000802000, 0x0290808050002000, 0x408a000a120020c1,
	0x8002000804120060, 0x0000800401800200, 0x000100010002004c, 0x8008800700104180,
	0x0010410020800501, 0x1000820020c11200, 0x8221030010492000, 0x0110010030092101,
	0x0004028004801800, 0x0002008100040080, 0x0080840048069001, 0x070042001c008141,
	0x0411800080234004, 0x4050024640002004, 0x0000100080802000, 0x0004200900100102,
	0x0000050100100800, 0x0202008a00240810, 0x0008480400020310, 0x00080052000103a4,
	0x10914003a8800880, 0x0481201000400049, 0x0001004071002001, 0x0105024b21001000,
	0x0084009482800800, 0x0101040080802200, 0x8820180224001003, 0x0806108406000841,
	0x9000400020808000, 0x0488200050004004, 0x4026120240820020, 0x0010000800808030,
	0x80010028000d0010, 0x0800040002008080, 0x0000088106040050, 0x8080128241320004,
	0x008000a00a400040, 0x20082502804a0600, 0x1020805000200080, 0x8090080110008080,
	0x2004008005880080, 0x201822000c008080, 0x4005000402000100, 0x00004a8044010600,
	0x1820800060403109, 0x2000220015830042, 0x4048205812008042, 0x0001a861001c1001,
	0xc006002004100816, 0x7902001028148116, 0x1040221590110804, 0x4011008028440302,
}

// ================================================================
// OCCUPANCY MASK GENERATION
// ================================================================
//
//   Edges are excluded: a blocker on the edge never changes the
//   attack set, so it contributes nothing to the occupancy hash.
//

func rookMask(sq int) uint64 {
	var mask uint64
	r, f := sq/8, sq%8
	for rr := 1; rr < 7; rr++ {
		if rr != r {
			mask |= squareBit(rr*8 + f)
		}
	}
	for ff := 1; ff < 7; ff++ {
		if ff != f {
			mask |= squareBit(r*8 + ff)
		}
	}
	return mask
}

func bishopMask(sq int) uint64 {
	var mask uint64
	r, f := sq/8, sq%8
	for i := 1; r+i < 7 && f+i < 7; i++ {
		mask |= squareBit((r+i)*8 + (f + i))
	}
	for i := 1; r+i < 7 && f-i > 0; i++ {
		mask |= squareBit((r+i)*8 + (f - i))
	}
	for i := 1; r-i > 0 && f+i < 7; i++ {
		mask |= squareBit((r-i)*8 + (f + i))
	}
	for i := 1; r-i > 0 && f-i > 0; i++ {
		mask |= squareBit((r-i)*8 + (f - i))
	}
	return mask
}

// ================================================================
// REFERENCE RAY-CASTING
// ================================================================
//
//   Used only during initMagics to fill the attack tables.
//   Never called during search.
//

func slideAttacksRef(sq int, occ uint64, dr, df [4]int) uint64 {
	var atk uint64
	r, f := sq/8, sq%8
	for i := 0; i < 4; i++ {
		nr, nf := r+dr[i], f+df[i]
		for nr >= 0 && nr < 8 && nf >= 0 && nf < 8 {
			s := nr*8 + nf
			atk |= squareBit(s)
			if occ&squareBit(s) != 0 {
				break
			}
			nr += dr[i]
			nf += df[i]
		}
	}
	return atk
}

func rookAttacksRef(sq int, occ uint64) uint64 {
	return slideAttacksRef(sq, occ, [4]int{1, -1, 0, 0}, [4]int{0, 0, 1, -1})
}

func bishopAttacksRef(sq int, occ uint64) uint64 {
	return slideAttacksRef(sq, occ, [4]int{1, 1, -1, -1}, [4]int{1, -1, 1, -1})
}

// ================================================================
// TABLE FILLING
// ================================================================
//
//   Carry-Rippler subset enumeration visits every subset of mask
//   exactly once: sub = (sub - mask) & mask.
//

// tryFill fills one square's attack table.  Returns false on a
// destructive collision (wrong magic number for this square).
func tryFill(isBishop bool, sq int, magic, mask uint64, shift uint, table []uint64) bool {
	for i := range table {
		table[i] = 0
	}
	sub := uint64(0)
	for {
		var attacks uint64
		if isBishop {
			attacks = bishopAttacksRef(sq, sub)
		} else {
			attacks = rookAttacksRef(sq, sub)
		}
		idx := (sub * magic) >> shift
		if table[idx] == 0 {
			table[idx] = attacks
		} else if table[idx] != attacks {
			return false // destructive collision
		}
		sub = (sub - mask) & mask
		if sub == 0 {
			break
		}
	}
	return true
}

// ================================================================
// INITIALIZATION
// ================================================================

func initMagics() {
	bOff, rOff := 0, 0
	for sq := 0; sq < 64; sq++ {
		bMask := bishopMask(sq)
		bBits := popCount(bMask)
		bSize := 1 << bBits
		bShift := uint(64 - bBits)
		bishopMagics[sq] = magicEntry{
			mask:   bMask,
			magic:  magicsBishop[sq],
			shift:  bShift,
			offset: bOff,
		}
		if !tryFill(true, sq, magicsBishop[sq], bMask, bShift, bishopFlat[bOff:bOff+bSize]) {
			panic("initMagics: bad bishop magic")
		}
		bOff += bSize

		rMask := rookMask(sq)
		rBits := popCount(rMask)
		rSize := 1 << rBits
		rShift := uint(64 - rBits)
		rookMagics[sq] = magicEntry{
			mask:   rMask,
			magic:  magicsRook[sq],
			shift:  rShift,
			offset: rOff,
		}
		if !tryFill(false, sq, magicsRook[sq], rMask, rShift, rookFlat[rOff:rOff+rSize]) {
			panic("initMagics: bad rook magic")
		}
		rOff += rSize
	}
}

// ================================================================
// ATTACK LOOKUPS
// ================================================================

func bishopAttacks(occ uint64, sq int) uint64 {
	m := &bishopMagics[sq]
	return bishopFlat[m.offset+int((occ&m.mask)*m.magic>>m.shift)]
}

func rookAttacks(occ uint64, sq int) uint64 {
	m := &rookMagics[sq]
	return rookFlat[m.offset+int((occ&m.mask)*m.magic>>m.shift)]
}

func queenAttacks(occ uint64, sq int) uint64 {
	return bishopAttacks(occ, sq) | rookAttacks(occ, sq)
}

// ================================================================
// MAGIC NUMBER GENERATOR
// ================================================================
//
//   Sparse random candidates (AND of three randoms) find valid
//   magics quickly.  Invoked via:  .\rodent_v.exe genmagics
//

func findMagicsForPiece(isBishop bool) {
	label := "Rook"
	if isBishop {
		label = "Bishop"
	}
	fmt.Printf("var magics%s = [64]uint64{\n", label)

	scratch := make([]uint64, 4096)
	rng := rand.New(rand.NewSource(0xDEADBEEF))

	for sq := 0; sq < 64; sq++ {
		var mask uint64
		if isBishop {
			mask = bishopMask(sq)
		} else {
			mask = rookMask(sq)
		}
		n := popCount(mask)
		shift := uint(64 - n)
		size := 1 << n

		var magic uint64
		for {
			magic = rng.Uint64() & rng.Uint64() & rng.Uint64()
			if tryFill(isBishop, sq, magic, mask, shift, scratch[:size]) {
				break
			}
		}
		if sq%4 == 0 {
			fmt.Print("\t")
		}
		fmt.Printf("0x%016x,", magic)
		if sq%4 == 3 {
			fmt.Print("\n")
		} else {
			fmt.Print(" ")
		}
	}
	fmt.Print("}\n\n")
}

// FindMagics generates and prints new magic numbers for bishops and rooks.
// Invoked via:  .\rodent_v.exe genmagics
// Paste the output over magicsBishop / magicsRook above.
func FindMagics() {
	fmt.Println("// Generated by: .\\rodent_v.exe genmagics")
	fmt.Println("// Paste over magicsBishop and magicsRook in magics.go")
	findMagicsForPiece(true)
	findMagicsForPiece(false)
}
