# Rodent V — Roadmap

## Goal for first release

- **3200 Elo CCRL**
- Ability to define playing styles ("personalities") in a text file
- Weakening in a sensible range (not necessarily to absolute beginner level)
- HCE eval (for style) supplemented by an auxiliary neural network (for strength boost)

---

## Milestone: Basic Search

- [x] Strict PV node separation in hashing
- [x] Late Move Reduction (one-ply, non-PV nodes)
- [ ] Full draw detection (50-move rule, insufficient material)
- [ ] Modern PVS (no beta condition)
- [ ] History heuristic improvements (like in Chal)

---

## Milestone: Basic Eval

- [ ] King safety
- [ ] Doubled and backward pawns
- [ ] King pawn shield

---

## Milestone: Texel Tuner Port

A minimal port of the Texel tuner — not for full-blown tuning, but for
initial assessment of eval term additions.

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

## Longer term

- NNUE auxiliary network for strength boost alongside HCE
- Singular extensions
- Online play integration (Go's HTTP support makes this natural)
