# ELO Results

Self-play testing results across versions. All tests use **fastchess** with
`8+0.08` time control, 16 MB hash, `UHO_Lichess_4852_v1.epd`
opening suite. H0=0, H1=10.

---

## 0.2.0 — Late Move Reduction

**New vs 0.1.x base**

```
Elo: 58.69 +/- 23.22, nElo: 77.95 +/- 30.21
LOS: 100.00 %, DrawRatio: 37.80 %, PairsRatio: 2.16
Games: 508, Wins: 218, Losses: 133, Draws: 157, Points: 296.5 (58.37 %)
Ptnml(0-2): [12, 38, 96, 69, 39], WL/DD Ratio: 2.84
LLR: 2.92 (100.9%) (-2.25, 2.89) [0.00, 10.00]  PASSED
```

Change: one-ply LMR for quiet moves in non-PV nodes (depth > 2, movesTried > 3,
not in check).

---

## 0.1.0 — Tapered Evaluation

**New vs Sungorus base**

```
Elo: 190.85 +/- 47.50, nElo: 225.29 +/- 45.70
LOS: 100.00 %, DrawRatio: 27.03 %, PairsRatio: 9.12
Games: 222, Wins: 151, Losses: 40, Draws: 31, Points: 166.5 (75.00 %)
Ptnml(0-2): [4, 4, 30, 23, 50], WL/DD Ratio: 14.00
LLR: 2.90 (100.2%) (-2.25, 2.89) [0.00, 10.00]  PASSED
```

Change: mg/eg phase interpolation, PeSTo PSTs, mobility, passed pawns,
isolated pawns.

---

## Cumulative gain over Sungorus base

| Version | Feature              | Elo gain |
|---------|----------------------|----------|
| 0.1.0   | Tapered eval         | +191     |
| 0.2.0   | LMR                  | +59      |
| **Total**   |                  | **~+250** |
