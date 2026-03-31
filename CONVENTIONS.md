## Go style baseline

Follow standard Go style. When in doubt, ask: *what would `gofmt` do?*

- **Indentation**: tabs only - never spaces for indentation.
- **Braces**: opening brace on the same line, always.
- **`if` conditions**: no parentheses — `if !isReduced {`, not `if (!isReduced) {`.
- **Comments**: always a space after `//` — `// comment`, not `//comment`.
- **Semicolons**: never write them explicitly; Go's lexer inserts them.
- **Arithmetic spacing**: tight — `newDepth-1`, `ply+1`, not `newDepth - 1`.
- **Variable declarations**: no alignment padding with extra spaces in `:=` lines.
  ```go
  // good
  ttMove := 0
  ttFlag := 0
  score := 0

  // bad
  ttMove  := 0
  ttFlag  := 0
  score   := 0
  ```

---

## Naming

| Thing | Convention | Example |
|-------|-----------|---------|
| Functions | `camelCase` | `evaluatePieces`, `initMovePicker` |
| Types | `PascalCase` | `MovePicker`, `EvalData`, `MoveGenStage` |
| Constants | `PascalCase` or `ALL_CAPS` for piece/color literals | `StageTTMove`, `White`, `NO_PC` |
| Package-level vars | `camelCase` | `histTable`, `passedMask` |
| Local vars | `camelCase`, short | `sq`, `mob`, `best`, `picker` |
| Boolean vars | prefix with `is`/`in`/`can` | `isPv`, `isRoot`, `nodeInCheck` |

---

## File structure

Each file owns one subsystem (see `main.go` table of contents). Prefer adding
code to the right existing file over creating a new one.

Global variables that are only used within a single file belong in that file's
`var ()` block or `init()`, not in `tables.go`. This keeps locality and reduces
the chance of merge conflicts when two people edit different subsystems.

```go
// eval.go — masks only used here go here, not in tables.go
var (
    passedMask  [2][64]uint64
    adjFileMask [8]uint64
)

func init() {
    // ... fill them
}
```

---

## Comments

- File-level doc blocks use the `// ===...=== // SN NAME` banner format already
  established. Keep that style for any new file.
- Function comments: one short sentence saying *what* it returns/does.
  Add *why* only when the logic is non-obvious.
- Don't comment out dead code — delete it. Git remembers.

---

## Numeric tables (PSTs, bonuses)

PST arrays are formatted for visual readability of the chess values, two ranks
per line with a `/* piece rankN-rankM */` inline comment. This intentionally
diverges from `gofmt`'s default. Do not reformat them with a tool.

---

## Commit messages

Follow the style already in `history.txt`:

```
[feat search] description of what changed
[feat eval]   ...
[refactor]    ...
[perf]        ...
[tweak]       ...
[fix]         ...
[test]        ...
[chore]       ...
```

Version bumps go in `history.txt` with test results attached when available.
