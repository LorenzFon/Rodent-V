package main

type EvalHashEntry struct {
	key   uint64
	score int
	used  bool
}

var evalTT []EvalHashEntry

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