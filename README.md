# Rodent V

A UCI chess engine written in Go by **Naman Thanki** and **Pawel Koziol**.

Based on **Sungorus 1.4** by Pablo Vazquez.

---

## Features

- Precomputed sliding attack tables (rank, file, diagonal, anti-diagonal): O(1) lookups
- Iterative deepening with Principal Variation Search (PVS)
- Tapered evaluation: midgame/endgame interpolation via game phase
- PeSTo piece-square tables (modified for pawn eval coexistence)
- Null-move pruning (R=3, skipped in PV nodes and when in check)
- Late Move Reduction: one-ply reduction for quiet moves in non-PV nodes
- Quiescence search with Static Exchange Evaluation (SEE)
- 4-bucket transposition table with aging
- Killer heuristic (2 killers per ply)
- History heuristic
- Pawn structure: passed pawn bonuses, isolated pawn penalties
- Mobility evaluation: weighted by piece type, scaled mg/eg
- Full UCI support:
  - time controls (`wtime`/`btime`/`winc`/`binc`/`movestogo`)
  - `go depth`, `go movetime`, `go infinite`
  - pondering, `stop`
- NPS reporting in UCI `info` lines

---

## Build

```
go build -o rodent-v .
```

Requires **Go 1.21 or newer**.

---

## Run

Works with any **UCI GUI** (Arena, CuteChess, Banksia, etc.) or directly:

```
./rodent-v
uci
isready
position startpos
go depth 12
```

---

## UCI Options

| Name       | Default | Description           |
|------------|---------|-----------------------|
| Hash       | 16      | Hash table size in MB |
| Clear Hash | —       | Clears the hash table |

---

## Debug Commands

| Command     | Description                             |
|-------------|-----------------------------------------|
| `perft <n>` | Count leaf nodes at depth n             |
| `print`     | Print the current board to the terminal |

---

## Perft

```
position startpos
perft 5
```

Expected:

```
Nodes: 4865609
```

---

## Files

| File          | Section | Description                                        |
|---------------|---------|----------------------------------------------------|
| `tables.go`   | S1      | Constants, bit helpers, precomputed attack tables  |
| `pos.go`      | S2      | Board representation and FEN parsing               |
| `attacks.go`  | S3      | Attack detection                                   |
| `moves.go`    | S4      | Make / unmake move                                 |
| `gen.go`      | S5      | Move generation                                    |
| `legal.go`    | S6      | Move legality validation                           |
| `eval.go`     | S7      | Static evaluation (material, PST, mobility, pawns) |
| `next.go`     | S8      | Move ordering (TT → good caps → killers → quiet)   |
| `trans.go`    | S9      | Transposition table (4-bucket hash with aging)     |
| `swap.go`     | S10     | Static Exchange Evaluation (SEE)                   |
| `search.go`   | S11     | Principal Variation Search + quiescence            |
| `uci.go`      | S12     | UCI protocol, time management, perft               |
| `bitboard.go` | —       | Bitboard shift helpers                             |
| `main.go`     | —       | Entry point                                        |

---

## ELO Progress

See [ELO.md](ELO.md).

## Roadmap

See [ROADMAP.md](ROADMAP.md).

---

## Credits

- **Sungorus 1.4** — Pablo Vazquez (original C engine)
- **Rodent V** — Naman Thanki and Pawel Koziol
