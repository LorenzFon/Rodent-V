package main

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

type dgEntry struct {
	fen   string
	eval  int // White perspective
	score float64
}

var datagenMode bool = false

func runDatagen(targetPositions, threads, nodesPerMove int, bookFile string) {
	fmt.Printf("Starting datagen: %d target positions, %d threads, %d nodes/move\n", targetPositions, threads, nodesPerMove)
	allocTT(16)
	datagenMode = true

	var bookFENs []string
	if bookFile != "" {
		bf, err := os.Open(bookFile)
		if err == nil {
			scanner := bufio.NewScanner(bf)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					bookFENs = append(bookFENs, line)
				}
			}
			bf.Close()
			fmt.Printf("Loaded %d positions from book %s\n", len(bookFENs), bookFile)
		} else {
			fmt.Printf("Warning: could not open book file %s, using startpos\n", bookFile)
		}
	}

	filename := "data.txt"
	var totalPositions int64

	if _, err := os.Stat(filename); err == nil {
		fmt.Printf("Found existing %s, counting positions to resume...\n", filename)
		count, _ := countLines(filename)
		totalPositions = count
		fmt.Printf("Resuming from %d positions.\n", totalPositions)
	}

	if totalPositions >= int64(targetPositions) {
		fmt.Println("Target positions already reached!")
		return
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening output file: %v\n", err)
		return
	}
	defer file.Close()

	var fileMutex sync.Mutex
	var totalGames int64
	startPositions := totalPositions
	startTime := time.Now()
	var wg sync.WaitGroup

	// Progress monitor
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fileMutex.Lock()
				p := totalPositions
				g := totalGames
				fileMutex.Unlock()
				elapsed := time.Since(startTime).Seconds()
				posPerSec := float64(p-startPositions) / elapsed
				if elapsed == 0 {
					posPerSec = 0
				}
				fmt.Printf("Progress: %d / %d positions, %d games, %.0f pos/sec\n", p, targetPositions, g, posPerSec)
			case <-done:
				return
			}
		}
	}()

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(threadID)))
			ss := new(SearchState)
			ss.isUsingNNUE = nnue.Loaded && singleOptions[NnuePerc] > 0

			for {
				fileMutex.Lock()
				currentTotal := totalPositions
				fileMutex.Unlock()

				if currentTotal >= int64(targetPositions) {
					return
				}

				entries, played := dgPlayGame(rng, ss, nodesPerMove, bookFENs)
				if !played || len(entries) == 0 {
					continue
				}

				fileMutex.Lock()
				for _, e := range entries {
					resStr := "0.5"
					if e.score > 0.9 {
						resStr = "1.0"
					} else if e.score < 0.1 {
						resStr = "0.0"
					}
					// FEN | eval | result
					file.WriteString(fmt.Sprintf("%s | %d | %s\n", e.fen, e.eval, resStr))
				}
				totalPositions += int64(len(entries))
				totalGames++
				fileMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()
	close(done)
	fmt.Printf("Datagen complete. Total games: %d, Total positions: %d\n", totalGames, totalPositions)
}

func dgPlayGame(rng *rand.Rand, ss *SearchState, nodesPerMove int, bookFENs []string) ([]dgEntry, bool) {
	var p Pos

	numRandom := 0
	if len(bookFENs) > 0 {
		fen := bookFENs[rng.Intn(len(bookFENs))]
		parseFEN(&p, fen)
		numRandom = rng.Intn(10) // 0 to 9 random moves from book pos
	} else {
		parseFEN(&p, startFEN)
		numRandom = 8 + rng.Intn(3) // 8 to 10 random moves from start pos
	}

	for i := 0; i < numRandom; i++ {
		var list [maxMoves]int
		capCount := genCaptures(&p, list[:])
		quietCount := genQuiet(&p, list[capCount:])
		total := capCount + quietCount

		var legals []int
		for j := 0; j < total; j++ {
			move := list[j]
			var child Pos = p
			var u Update
			var r Revert
			makeMove(&child, &u, &r, move)
			if !child.selfInCheck() {
				legals = append(legals, move)
			}
		}
		if len(legals) == 0 {
			return nil, false
		}
		move := legals[rng.Intn(len(legals))]
		var u Update
		var r Revert
		makeMove(&p, &u, &r, move)
		if p.clock == 0 {
			p.histLen = 0
		}
	}

	// Make sure NNUE is ready
	refresh(&p, &ss.accStack[0])
	ss.clearHistory()

	var entries []dgEntry
	drawCount := 0
	var result float64 = 0.5 // 0.5 for draw, 1.0 for White win, 0.0 for Black win

	for ply := 0; ply < 512; ply++ {
		var list [maxMoves]int
		capCount := genCaptures(&p, list[:])
		quietCount := genQuiet(&p, list[capCount:])
		total := capCount + quietCount

		hasLegal := false
		for j := 0; j < total; j++ {
			move := list[j]
			var child Pos = p
			var u Update
			var r Revert
			makeMove(&child, &u, &r, move)
			if !child.selfInCheck() {
				hasLegal = true
				break
			}
		}

		if !hasLegal {
			if p.inCheck() {
				if p.side == White {
					result = 0.0 // Black wins
				} else {
					result = 1.0 // White wins
				}
			}
			break
		}

		if p.isInsufficientMaterial() || p.clock >= 100 || isRepetitionDG(&p) {
			result = 0.5
			break
		}

		bestMove, score := runDatagenSearch(&p, ss, nodesPerMove)
		if bestMove == 0 {
			break
		}

		// FILTER QUIET POSITIONS
		isQuiet := true
		if p.inCheck() || isMateScore(score) {
			isQuiet = false
		} else {
			staticScore := evaluate(&p, &ss.accStack[0])
			var pv [maxPly]int
			qsScore := ss.quiesce(&p, 0, -inf, inf, pv[:])

			diff := staticScore - qsScore
			if diff < 0 {
				diff = -diff
			}
			if diff >= 50 {
				isQuiet = false
			}
		}

		if isQuiet {
			whiteScore := score
			if p.side == Black {
				whiteScore = -score
			}

			entries = append(entries, dgEntry{
				fen:   p.generateFen(),
				eval:  whiteScore,
				score: 0,
			})
		}

		if score >= -10 && score <= 10 {
			drawCount++
		} else {
			drawCount = 0
		}
		if ply >= 40 && drawCount >= 10 {
			result = 0.5
			break
		}

		var u Update
		var r Revert
		makeMove(&p, &u, &r, bestMove)
		if p.clock == 0 {
			p.histLen = 0
		}
		refresh(&p, &ss.accStack[0])
	}

	for i := range entries {
		entries[i].score = result
	}

	return entries, true
}

func isRepetitionDG(p *Pos) bool {
	end := p.histLen - p.clock
	if end < 0 {
		end = 0
	}
	for i := p.histLen - 2; i >= end; i -= 2 {
		if p.key == p.keyHist[i] {
			return true
		}
	}
	return false
}

func runDatagenSearch(p *Pos, ss *SearchState, softNodesLimit int) (int, int) {
	ss.resetForSearch(p)
	
	// HARD nodes limit checked mid-search (8 million nodes)
	ss.nodesLimit = 8000000
	refresh(p, &ss.accStack[0])

	var pv [maxPly]int
	score := 0
	bestMove := 0

	for d := 1; d < 100; d++ {
		iterScore := ss.search(p, 0, -inf, inf, d, false, pv[:])
		
		// If we hit the 8M hard limit mid-search, we discard this depth's result
		if ss.isAbortingSearch() {
			break
		}
		
		if pv[0] != 0 {
			score = iterScore
			bestMove = pv[0]
		}
		
		// SOFT nodes limit checked only AFTER an iteration finishes
		if ss.nodes >= int64(softNodesLimit) {
			break
		}
	}
	
	// Reset the nodes limit to 0 so the subsequent quiesce filter doesn't instantly abort
	ss.nodesLimit = 0
	
	return bestMove, score
}

func countLines(filename string) (int64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var count int64
	buf := make([]byte, 32*1024)
	for {
		c, err := file.Read(buf)
		count += int64(bytes.Count(buf[:c], []byte{'\n'}))
		if err != nil {
			break
		}
	}
	return count, nil
}
