# Rodent V — Roadmap

## Goal for first release

- **3200 Elo CCRL** (is 2600-2700)
- Ability to define playing styles ("personalities") in a text file
- Weakening in a sensible range (not necessarily to absolute beginner level)
- HCE eval (for style) supplemented by an auxiliary neural network (for strength boost)

---

## Milestone: Basic Search

- [x] Strict PV node separation in hashing
- [x] Late Move Reduction (one-ply, non-PV nodes)
- [x] Full draw detection (50-move rule, insufficient material)
- [ ] Modern PVS (no beta condition)
- [x] History heuristic improvements (like in Chal)

---

## Milestone: Basic Eval

- [x] King safety
- [x] Doubled pawns
- [ ] Backward pawns
- [x] King pawn shield

---

## Milestone: Tuning round 1

A minimal port of the Texel tuner — not for full-blown tuning, but for
initial assessment of eval term additions.
- [x] minimal tuner to assess new eval terms
- [x] Texel tuner with batches
- [x] tune pst
- [x] tune passers
- [ ] tune threats
- [ ] tune mobility
- [ ] tune material (risky)
- [ ] tune whatever remains

---

## Milestone: Easy Search Improvements

- [ ] Reverse futility pruning (RFP)
- [ ] Late move pruning (LMP)
- [ ] Table-driven LMR
- [ ] Razoring
- [ ] SEE fast-path: `isBadCapture` short-circuits for obviously good captures
      (attacker value ≤ victim value, BxN) before running full SEE

---

## Milestone: Asymmetric Eval + Personalities

- [ ] Separate options for own vs. opponent attack and mobility weights
- [ ] Define basic option list accessible for personality tuning
- [ ] Load personalities from a text file at startup

---

## Milestone: Regaining eval speed

- [ ] Eval hashtable
- [ ] Separating eval functions related only to pawns and kings
- [ ] Pawn hashtable

---

## Milestone: Small search gains (expect long tuning runs)

- [ ] mate distance pruning
- [ ] futility pruning

---

## Milestone: advanced search additions

- [ ] Singular extensions
- [ ] refutaton history
- [ ] continuation history if does not fail
- [ ] correction history if does not fail

---

## Milestone: tuning round 2

- [ ] Multi-threaded tuner
- [ ] Retune everything with a better set

---

## Milestone: better quiescence search

- [ ] Direct checking moves generator
- [ ] Discovered checks generator
- [ ] (possibly) out of check move generator

## Longer term

- NNUE auxiliary network for strength boost alongside HCE
- Singular extensions
- Online play integration (Go's HTTP support makes this natural)
