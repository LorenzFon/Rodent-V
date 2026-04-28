package main

type EvalHashEntry struct {
	key   uint64
	score int
	used  bool
}

type PawnHashEntry struct {
	key   uint64
	scoreMG[2] int
	scoreEG[2] int
	center[2] int
	used  bool
}

var evalTT []EvalHashEntry
var pawnTT []PawnHashEntry

func initEvalHash(size int) {
	if size <= 0 {
		evalTT = nil
		return
	}
	evalTT = make([]EvalHashEntry, size)
}

func clearEvalHash() {
	for i := range evalTT {
		evalTT[i] = EvalHashEntry{}
	}
}

func probeEvalHash(key uint64) (int, bool) {
	if len(evalTT) == 0 {
		return 0, false
	}

	e := evalTT[key%uint64(len(evalTT))]
	if e.used && e.key == key {
		return e.score, true
	}
	return 0, false
}

func storeEvalHash(key uint64, score int) {
	if len(evalTT) == 0 {
		return
	}
	evalTT[key%uint64(len(evalTT))] = EvalHashEntry{
		key:   key,
		score: score,
		used:  true,
	}
}

// --- Pawn hash ---

func initPawnHash(size int) {
	if size <= 0 {
		pawnTT = nil
		return
	}
	pawnTT = make([]PawnHashEntry, size)
}

func clearPawnHash() {
	for i := range evalTT {
		pawnTT[i] = PawnHashEntry{}
	}
}

// TODO: make it a function operating on EvalData, too many returns
func probePawnHash(key uint64) (int, int, int, int, int, int, bool) {
	if len(pawnTT) == 0 {
		return 0, 0, 0, 0, int(Undefined), int(Undefined), false
	}

	e := pawnTT[key%uint64(len(pawnTT))]
	if e.used && e.key == key {
		return e.scoreMG[White], e.scoreMG[Black], e.scoreEG[White], e.scoreEG[Black], e.center[White], e.center[Black], true
	}
	return 0, 0, 0, 0, int(Undefined), int(Undefined), false
}

func storePawnHash(key uint64, wscoreMG, bscoreMG, wscoreEG, bscoreEG, wCenter, bCenter int) {
	if len(evalTT) == 0 {
		return
	}

	addr := key%uint64(len(pawnTT))

    pawnTT[addr].key = key
	pawnTT[addr].scoreMG[White] = wscoreMG
	pawnTT[addr].scoreMG[Black] = bscoreMG
	pawnTT[addr].scoreEG[White] = wscoreEG
	pawnTT[addr].scoreEG[Black] = bscoreEG
	pawnTT[addr].center[White] = wCenter
	pawnTT[addr].center[Black] = bCenter
	pawnTT[addr].used = true
}