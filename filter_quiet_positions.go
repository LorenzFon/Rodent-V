package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

const (
	maxEvalQSDifference = 50
	progressInterval    = 50000
)

// filterQuietBulletFile copies only positions that are not in check and
// satisfy abs(static evaluation - quiescence score) < 50.
//
// Input and output use the old bullet format:
//
//	FEN | evaluation | result
//
// Accepted lines are written unchanged.
func filterQuietBulletFile(
	inputPath string,
	outputPath string,
) error {
	in, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer in.Close()

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	writer := bufio.NewWriter(out)

	lineNumber := 0
	positionCount := 0
	acceptedCount := 0
	inCheckCount := 0
	noisyCount := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "|")
		if len(fields) < 3 {
			return fmt.Errorf(
				"line %d: expected at least three pipe-separated fields: %q",
				lineNumber,
				line,
			)
		}

		fen := strings.TrimSpace(fields[0])

		result, err := testQuietPosition(fen)
		if err != nil {
			return fmt.Errorf(
				"line %d, FEN %q: %w",
				lineNumber,
				fen,
				err,
			)
		}

		positionCount++

		switch result {
		case filterAccepted:
			if _, err := fmt.Fprintln(writer, line); err != nil {
				return fmt.Errorf("write line %d: %w", lineNumber, err)
			}
			acceptedCount++

		case filterInCheck:
			inCheckCount++

		case filterNoisy:
			noisyCount++
		}

		if positionCount%progressInterval == 0 {
			if err := writer.Flush(); err != nil {
				return fmt.Errorf(
					"flush after %d positions: %w",
					positionCount,
					err,
				)
			}

			fmt.Printf(
				"checked %d, accepted %d (%.1f%%), noisy %d, in check %d\n",
				positionCount,
				acceptedCount,
				percentage(acceptedCount, positionCount),
				noisyCount,
				inCheckCount,
			)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan input: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush output: %w", err)
	}

	fmt.Printf(
		"filter complete: checked %d, accepted %d (%.1f%%), noisy %d, in check %d\n",
		positionCount,
		acceptedCount,
		percentage(acceptedCount, positionCount),
		noisyCount,
		inCheckCount,
	)

	return nil
}

type filterResult uint8

const (
	filterAccepted filterResult = iota
	filterInCheck
	filterNoisy
)

func testQuietPosition(fen string) (filterResult, error) {
	var p Pos
	parseFEN(&p, fen)

	// Static evaluation is not meaningful for this filter while the side
	// to move is in check.
	if p.inCheck() {
		return filterInCheck, nil
	}

	var ss SearchState
	ss.resetForSearch(&p)
	refresh(&p, &ss.accStack[0])

	atomic.StoreInt32(&abortFlag, 0)
	hardTimeLimit = -1
	singleOptions[NodesLimit] = 0

	staticScore := evaluate(&p, &ss.accStack[0])

	var pv [maxPly]int
	qsScore := ss.quiesce(
		&p,
		0,
		-inf,
		inf,
		pv[:],
	)

	if absInt(staticScore-qsScore) >= maxEvalQSDifference {
		return filterNoisy, nil
	}

	return filterAccepted, nil
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func percentage(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(part) / float64(total)
}
