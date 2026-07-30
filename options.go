// ================================================================
//
//	R O D E N T   V
//
// ================================================================
//
//	A Go chess engine by Naman Thanki and Pawel Koziol.
//	Based on Sungorus 1.4 by Pablo Vazquez (2013).
//
//	Authors        : Naman Thanki, Pawel Koziol
//	Date           : 2026
//
//	Every file is a short lesson in chess engine design. Follow the
//	table of contents below to understand the full pipeline from
//	board representation to UCI output.
//
//	Protocol: Universal Chess Interface (UCI)
//	Build:    go build -o rodent-v .

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var nnuePath string // default path to NNUE file

var pestoEval bool            // are we using pesto eval?
var adjustEvalByCorrhist bool // are we using corrhist
// (corrhist makes engine stronger but can mask eval properties)

// color-independent options
type SingleOption int

const (
	HcePerc SingleOption = iota
	NnuePerc
	NodesLimit
	NofSingleOptions
)

var SingleOptionName = [NofSingleOptions]string{
	HcePerc:    "hceWeight",
	NnuePerc:   "nnueWeight",
	NodesLimit: "nodesLimit",
}

var singleOptions [NofSingleOptions]int

// color-dependent options need to be indexed
// by engine/non engine side, not white/black
var engineSide int

const weightOwn = 0
const weightOpp = 1

// NOTE: evalComponent is defined in EvalData
var optionPerColorValues [2][EvalComponentN]int

// default settings
func init() {

	nnuePath = "nets/rodent_v_256hl_4.bin"
	singleOptions[NnuePerc] = 100
	singleOptions[HcePerc] = 0
	singleOptions[NodesLimit] = 0

	pestoEval = false
	adjustEvalByCorrhist = true

	for c := EvalComponent(0); c < EvalComponentN; c++ {
		optionPerColorValues[weightOwn][c] = 100
		optionPerColorValues[weightOpp][c] = 100
	}
}

// saveOptions writes the current engine options as UCI setoption commands,
// one option per line.
func saveOptions(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create options file %q: %w", path, err)
	}

	fmt.Println("info string saving personality ", path)

	writer := bufio.NewWriter(file)

	writeErr := func() error {
		// Header comments are ignored by readOptions.
		if _, err := fmt.Fprintln(writer, "; Rodent V options"); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(
			writer,
			"setoption name nnuePath value %s\n",
			nnuePath,
		); err != nil {
			return err
		}

		for option := SingleOption(0); option < NofSingleOptions; option++ {
			if _, err := fmt.Fprintf(
				writer,
				"setoption name %s value %d\n",
				SingleOptionName[option],
				singleOptions[option],
			); err != nil {
				return err
			}
		}

		for component := EvalComponent(0); component < EvalComponentN; component++ {
			if _, err := fmt.Fprintf(
				writer,
				"setoption name Own%s value %d\n",
				evalComponentName[component],
				optionPerColorValues[weightOwn][component],
			); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(
				writer,
				"setoption name Opp%s value %d\n",
				evalComponentName[component],
				optionPerColorValues[weightOpp][component],
			); err != nil {
				return err
			}
		}

		return writer.Flush()
	}()

	closeErr := file.Close()

	if writeErr != nil {
		return fmt.Errorf("write options file %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close options file %q: %w", path, closeErr)
	}

	return nil
}

// readOptions reads UCI setoption commands from a file.
//
// Empty lines and lines whose first non-space character is ';' are ignored.
// Unknown or malformed commands are skipped. Option values may contain spaces.
func readOptions(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open options file %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Permit long paths or future string options.
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		line := strings.TrimSpace(scanner.Text())

		// Remove a possible UTF-8 BOM from the beginning of the file.
		if lineNumber == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}

		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		tokens := strings.Fields(line)
		if len(tokens) == 0 {
			continue
		}

		if !strings.EqualFold(tokens[0], "setoption") {
			continue
		}

		// parseSetOption expects everything after the "setoption" token.
		parseSetOption(tokens[1:])
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read options file %q: %w", path, err)
	}

	return nil
}

// prints color-separated options
func printUciOptionsPerColor() {
	for c := EvalComponent(0); c < EvalComponentN; c++ {
		fmt.Printf(
			"option name Own%s type spin default %d min 0 max 500\n",
			evalComponentName[c],
			optionPerColorValues[weightOwn][c],
		)
		fmt.Printf(
			"option name Opp%s type spin default %d min 0 max 500\n",
			evalComponentName[c],
			optionPerColorValues[weightOpp][c],
		)
	}
}
