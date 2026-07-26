# Next growth ideas (post easy-wins)

Follow-ups after [`easy-wins-refactor.md`](./easy-wins-refactor.md). Not a full
reshape — pick items as product needs dictate. Prefer small PRs.

**Constraints (unchanged):**
- Acyclic imports (`gameconfig` → `grid`/`network` → systems → `states`)
- `network` ↛ `grid`
- Dirty → `LoadflowSystem` sole solver caller
- Leave `pkg/nr` alone unless a feature forces it

Each section below says **today → tomorrow**, **why**, and **files to touch**.

---

## Quick leftovers

### SpawnHouse comment

**Today** (`spawn.go`):

```go
// It attaches a HouseLoad component with P and Q sampled uniformly from [10, 20] kW.
func SpawnHouse(...)
```

**Tomorrow:** comment (and any ImGui copy) must say `[HouseLoadMinKW, HouseLoadMaxKW]`
(currently 1.5–3.0). No behaviour change — `RandLoadKW()` already uses those consts.

**Why:** Stale docs will make the next load-tuning pass “fix” the wrong range.

### `plans/loadflow.md` stub wording

**Today:** doc still says `LoadflowSolver (stub → Newton-Raphson…)` and
“stub” in the file-layout section.

**Tomorrow:** describe the real pipeline: `BuildYBus` → NR via `pkg/nr` + SuperLU,
Dirty-driven `LoadflowSystem`, history rings. Point at `solver.go` / `ybus.go`.

**Why:** Plans are onboarding; “stub” implies the math isn’t done.

### Optional E2 — `network/powerflow` subpackage

**Today:** one package owns graph + NR math:

```
game/components/network/
  network.go, state.go, history.go   ← graph / ECS join
  ybus.go, sparse.go, solver.go      ← load-flow math
```

**Tomorrow (only if still muddy):**

```
game/components/network/           ← ElectricalNetwork, Bus, Branch, NetworkLink, state, history
game/components/network/powerflow/ ← LoadflowSolver, YBus, SparseMatrix
```

- `powerflow` imports parent `network` (types only).
- `systems/loadflow` imports `network/powerflow` (or a thin re-export).
- **Do not** move `ElectricalNetwork` into the child (that forces cycles).

**Why:** Name “components/network” currently over-promises “pure data”; a
subpackage makes the math boundary obvious without renaming the whole module.

---

## 1. Blank-tile cost → procedural grid

### Today

`states/grid_state.go` `Enter`:

```go
for row := 0; row < cfg.GridRows; row++ {
    for col := 0; col < cfg.GridCols; col++ {
        grid.SpawnBlank(s.World(), grid.GridCoord{Col: col, Row: row})
    }
}
```

Each blank is a full ECS entity: `Position` + `Sprite` + layer + `Order` +
`GridObject{Kind: ToolNone}`. At 100×100 that is **10 000 entities** before the
player places anything. The engine renderer then walks all of them every frame.

`GridOccupancy.Cells` only tracks *placed* tools today (gen/house/line), not
blanks — blanks exist only so the playfield looks like a grid.

### Tomorrow

| Remove / stop | Replace with |
|---------------|--------------|
| Nested `SpawnBlank` loop in `Enter` | Nothing in ECS for empty cells |
| Relying on blank sprites for the board look | `renderGridBackground()` in `grid_overlays.go` (or a tiny `systems/gridbg`) that draws cell outlines / a tiled quad in screen or world space using `CellScreenRect` + camera |
| (optional later) `SpawnBlank` itself | Keep the helper only if tests need a single tile; otherwise delete |

**Occupancy stays the source of truth for placed entities.** Hover/select/
ghost already use `ScreenToCell` + `GridOccupancy` — they do not need blank
entities.

**Sketch for overlay draw** (same package as toolbar chrome):

```go
// Visible cell range from cam + playfield size, then:
for row := r0; row <= r1; row++ {
    for col := c0; col <= c1; col++ {
        x, y, w, h := grid.CellScreenRect(cam, grid.GridCoord{Col: col, Row: row})
        ui.Rect(...) // hairline border or checker fill
    }
}
```

Clip to visible cells only (don’t loop all 10k every frame).

### Why

- Cut entity count and sprite passes by ~orders of magnitude at large grids.
- Placement/wiring APIs unchanged (`Occupy` / `Attach` only for real tiles).

### Files

- `states/grid_state.go` — delete blank loop
- `states/grid_overlays.go` — add background draw (before hover/ghost)
- `game/components/grid/spawn.go` — deprecate/remove `SpawnBlank` if unused
- Tests: any that assumed blanks exist (likely none)

---

## 2. Island / multi-slack handling

### Today

`LoadflowSolver.Solve` (`solver.go`):

```go
// Require at least one slack bus (the network reference).
hasSlack := false
for _, bs := range state.Buses {
    if bs.Spec.Formulation == Slack { hasSlack = true; break }
}
if !hasSlack {
    return fmt.Errorf("loadflow: no slack bus in network")
}
yb := BuildYBus(net)  // one Y-bus for the entire graph
// … one NR solve over all free buses …
```

So:

- Two separate feeders, each with a generator → **two Slacks in one Y-bus**
  (ill-posed / wrong reference).
- House + lines with **no** generator → error string only in the logger
  (and only if you look); ImGui still shows `Converged: false` with no reason.
- A house electrically islanded from the slack still sits in the same solve
  (floating PQ bus) → bad conditioning / nonsense voltages.

`wiring.Attach` already builds the adjacency the solver needs
(`AddBranch` between cardinal neighbours). What is missing is **component
analysis** before building one global Y-bus.

### Tomorrow

| Today | Tomorrow |
|-------|----------|
| One global “any slack?” check | `islands := ConnectedComponents(net)` (BFS/DFS on `Neighbors`) |
| One `BuildYBus(net)` + one NR | For each island: pick/validate slack; `BuildYBus` restricted to that island’s buses **or** solve islands sequentially and merge results into `StaticState` |
| Generators all `SlackSpec` by default | Policy: first generator in an island is Slack; extra generators become PV (or second Slack rejected with a clear error) |
| Error only via `fmt.Errorf` return | Persist status (see #5): `"island 2: no slack"`, `"island 0: multiple slack"` |

**Example policy (simple, game-friendly):**

```go
for _, island := range components {
    slacks := busesIn(island, Slack)
    switch len(slacks) {
    case 0:
        // mark island buses V=NaN or leave previous; record error; skip NR
    case 1:
        SolveIsland(net, island) // subset ordering
    default:
        // demote extras to PV at NominalVoltageV, or fail with message
    }
}
```

Minimal first slice: **detect components + refuse multi-slack / no-slack per
island with a stored error**, still one solve only when exactly one island
with one slack (improves messaging without full multi-island NR).

### Why

- Players will place two separate grids; today’s model silently mis-models that.
- Sets up ImGui status (#5) with actionable text.

### Files

- `network/solver.go` — island loop / validation
- New `network/islands.go` — `ConnectedComponents(net) [][]BusID`
- `network/state.go` — optional `LastSolveError string` / per-bus island id
- Tests: two disconnected gen+load islands; island with load only

---

## 3. Placement vs select split

### Today

`systems/placement/placement.go` `Update` does everything:

1. `updateHover` every frame  
2. **C** clears tool / cancels line  
3. Toolbar click → change `PlacementState.Tool`  
4. `ToolNone` + click → `HasSelection` / `SelectedCell`  
5. Tool active → `placeSingle` / line path / `deleteCell` → `wiring.Attach`/`Detach`

One file (~170 LOC) is still the choke point for all pointer UX.

### Tomorrow

| Responsibility | Package / type | Owns |
|----------------|----------------|------|
| Hover + ToolNone select + C-clear (optional) | `systems/pointer` (or `select`) | `PlacementState.Hover*`, `HasSelection`, `SelectedCell`; maybe tool clear |
| Toolbar + place/delete/line | `systems/placement` | Tool changes, spawn, `wiring.*` |
| Electrical join | `systems/wiring` | unchanged |

**Schedule in `Enter`:**

```go
// Today:
Add(placement.NewPlacementSystem(...))

// Tomorrow:
Add(pointer.NewPointerSystem(...))     // hover + select first
Add(placement.NewPlacementSystem(...)) // places only if Tool != None
```

**Code move example:**

```go
// LEAVE in pointer/select:
placement.HoverCell / HoverValid
if tool == ToolNone { HasSelection = true; SelectedCell = HoverCell }

// LEAVE in placement:
handleToolbarClick, placeSingle, handleLineClick, deleteCell
// still calls wiring.Attach / Detach
```

`PlacementState` resource can stay in `grid` (shared). No need to rename it on
day one; optionally later split into `PointerState` + `PlacementToolState`.

### Why

- Next features (box select, “inspect mode” sticky, drag-move) shouldn’t each
  grow `placement.go`.
- Matches the mental model already implied by ToolNone vs tool-active.

### Files

- New `game/systems/pointer/pointer.go` (name flexible)
- Slim `placement/placement.go`
- `states/grid_state.go` — register both systems
- Tests: pointer sets selection without spawning; placement still attaches

---

## 4. Line as one entity (polyline branch)

### Today

Placing a line from A→B:

```go
path := grid.ManhattanPath(start, end)
for _, c := range path {
    e := grid.SpawnLineSegment(w, c)           // one entity per cell
    occupancy.Occupy(c, e)
    wiring.Attach(w, e, ToolLine, c, occupancy) // one BusJunction + branches
}
```

A 50-cell feeder ⇒ **50 junction buses** + ~50 branches. Wiring uses **per-cell
resistance** (`LineSegmentProps.ResistanceOhm` ≈ one 10 m cell) on the branch
toward each neighbour — electrically OK, but NR size is O(cells).

Delete must remove one cell at a time; there is no “the line” object.

### Tomorrow (conceptual replace)

| Today | Tomorrow |
|-------|----------|
| N entities, N junction buses | **1** line entity (or 1 per polyline stroke) covering cells `path[]` |
| `AddBranch` between every adjacent line cell | **One** branch between the two electrical endpoints (or between endpoint buses of whatever gen/house/line it touches), with `R = DefaultLineResistanceOhm * (len(path)-1)` |
| `GridOccupancy.Cells[c] = e` for each cell | Occupancy maps **each cell on the path → same entity** (or a side map `lineCells`) so hover/select/delete still work |
| `SpawnLineSegment` per cell | `SpawnLine(w, path)` + component e.g. `LinePath{Cells []GridCoord, ResistanceOhm float64}` |

**Wiring sketch:**

```go
// Instead of Attach per cell:
func AttachLine(w, e, path []GridCoord, occ) {
    // Option A (simplest electrically): no junction buses at all —
    // connect the network buses of the two endpoints (gen/house/other line)
    // with one branch R = cellR * hops.
    //
    // Option B: keep one bus for the line entity; branches from that bus
    // to each distinct neighbour network bus at the endpoints only.
}
```

**Why this is harder:** delete-one-cell of a polyline, T-junctions (third line
touching the middle), and Manhattan corner geometry all need a clear policy.
Do **after** #1 and #5 so you’re not debugging 10k blanks + silent solve fails
at the same time.

### Why

- 100-bus radial test today is mostly junctions; real play should be
  “~N houses + M line strokes”, not “N cells of cable”.

### Files

- `grid/spawn.go` — `SpawnLine` / `LinePath` component
- `systems/wiring` — path-aware attach/detach; stop per-cell junction spam
- `systems/placement` — `handleLineClick` spawns one entity
- `states/grid_imgui.go` — selection shows path length + total R
- Ghost draw already uses `ManhattanPath` — keep that for preview

---

## 5. Converge feedback in ImGui

### Today

`loadflow.Update`:

```go
err := s.solver.Solve(net)
if err != nil {
    logger.Logger.Errorf("grid-sim: loadflow failed: %v", err)
}
// …
net.ClearDirty()
```

ImGui (`grid_imgui.go`) only shows:

```go
w.Text("  Converged: %v", st.Converged)
w.Text("  Iterations: %d", st.Iterations)
```

The error string (`"loadflow: no slack bus in network"`, NR failure, etc.) is
**discarded** after the log line. Players with `DebugLoadflowLog == false` see
no reason.

### Tomorrow

| Today | Tomorrow |
|-------|----------|
| `err` only logged | Persist on network or state, e.g. `net.State.LastError string` (clear on success) |
| ImGui: Converged + Iterations | Also: `LastError`, maybe `LastSolveOK bool`, timestamp optional |
| (optional) history skip on no-slack | Unchanged logic, but UI explains why history didn’t move |

**Minimal code:**

```go
// state.go — on StaticState:
LastError string // empty if last Solve returned nil

// solver.go — every return path:
state.LastError = ""
// on error:
state.LastError = err.Error()
return err

// loadflow.go — no need to log at Error if UI shows it; Debugf is enough

// grid_imgui.go:
if st.LastError != "" {
    w.Text("  Error: %s", st.LastError)
}
```

### Why

- Tiny change, high clarity; unblocks island work (#2) messaging.
- Avoids turning `DebugLoadflowLog` back on for normal play.

### Files

- `network/state.go`, `network/solver.go`
- `systems/loadflow/loadflow.go` (optional log level tweak)
- `states/grid_imgui.go`
- Test: no-slack solve → `LastError` non-empty, ImGui path covered indirectly

---

## 6. Line reactance (R+X)

### Today

`Branch` is resistive only:

```go
type Branch struct {
    ...
    Resistance float64 // Ω
}
```

`BuildYBus` already has a sparse **B** matrix but fills only **G**:

```go
g := 1.0 / r
// x (reactance) = 0 for purely resistive branches → b = 0
G.Add(...)
// B never gets series susceptance terms
```

`LineSegmentProps` only has `ResistanceOhm`. Contacts use `R=0` → clamped to
`minResistance`.

### Tomorrow

| Today | Tomorrow |
|-------|----------|
| `Branch.Resistance` | `Branch.Resistance` + `Branch.Reactance` (X in Ω) |
| `AddBranch(from, to, r)` | `AddBranch(from, to, r, x)` or `AddBranch(..., BranchParams{R, X})` |
| `LineSegmentProps{ResistanceOhm}` | add `ReactanceOhm` (default from e.g. X/R ≈ 0.1…0.5 for LV cable, or 0 to preserve today’s behaviour) |
| `BuildYBus`: `y = 1/r` | `y = 1/(r + j x)` → `g = r/(r²+x²)`, `b = -x/(r²+x²)`; fill **both** G and B |
| `wiring` line resistance only | pass X the same way as R for line tiles; contacts stay `X=0` |

**Default strategy:** ship `DefaultLineReactanceOhm = 0` (bit-identical to today),
then tune a small X/km constant next to `CableOhmPerKm` in `grid.go`.

### Why

- Enables realistic angle / Q flow demos without changing NR machinery
  (`CalcPQ` already uses G and B).
- Y-bus comment already anticipated `x = 0`.

### Files

- `network/network.go` — Branch + AddBranch signature (update all call sites/tests)
- `network/ybus.go` — complex series admittance
- `grid/grid.go` — props + defaults
- `systems/wiring` — read X from props
- Solver tests: small X feeder, check δ and Q non-zero

---

## 7. Save / load topology

### Today

World exists only in memory for the session:

- `GridOccupancy.Cells` → entity per cell  
- Components: `GridObject`, `HouseLoad`, `GeneratorProps`, `LineSegmentProps`  
- `ElectricalNetwork` graph rebuilt by `wiring.Attach` as you place  

No file format. Regression nets are hand-built in Go tests (`mustAddBus`…).

### Tomorrow

**Replace “only live ECS” with a round-trip DTO** (example JSON):

```json
{
  "version": 1,
  "cells": [
    {"col": 0, "row": 0, "kind": "generator"},
    {"col": 1, "row": 0, "kind": "line", "r_ohm": 0.00164},
    {"col": 2, "row": 0, "kind": "house", "p_kw": 2.1, "q_kw": 1.8}
  ]
}
```

| Load path | Steps |
|-----------|--------|
| Clear | Empty occupancy + new `ElectricalNetwork` (or remove placed entities) |
| Spawn | For each cell: `Spawn*` + `Occupy` + `wiring.Attach` (reuse game logic) |
| Specs | After attach, `SetBusSpec` for houses from saved P/Q |
| Solve | `MarkDirty` / let `LoadflowSystem` run |

| Save path | Steps |
|-----------|--------|
| Walk `GridOccupancy.Cells` | Read `GridObject.Kind` + props components |
| Do **not** need to serialize bus IDs | Re-attach regenerates IDs; history can reset |

**UI:** ImGui buttons “Save scenario…” / “Load…” or load a fixed
`assets/scenarios/demo.json` on a keybind for demos.

**Why not serialize `ElectricalNetwork` directly?** Bus IDs and entity handles
are session-local; replaying placement/wiring is the stable API and keeps
`network` ↛ file formats.

### Why

- Shareable demos, golden “place this → voltages within ε” tests without UI.
- Natural fixture for island / polyline work later.

### Files

- New `game/scenario/scenario.go` (encode/decode + `Apply(w)`)
- `states/grid_imgui.go` or a tiny system for save/load triggers
- `assets/scenarios/*.json`
- Test: apply fixture → Dirty → loadflow converges → spot-check \|V\|

---

## Skip for now

| Skip | Why |
|------|-----|
| Menu / multi-state scaffolding | One `GridState` is enough until there is a real meta-game |
| Rewriting SuperLU / NR | Solver stack is tested and fast enough; growth is model/UX |
| Full ImGui redesign | Status string (#5) beats a layout overhaul |

---

## Suggested order if doing a streak

1. Leftovers (comments/docs) — minutes  
2. **#5** ImGui solve status — small, unlocks clearer failures  
3. **#1** Procedural blank grid — medium, high payoff  
4. **#2** Island / multi-slack — medium (uses #5)  
5. **#3** Placement/select split — small–medium  
6. **#6** R+X or **#7** save/load — as needed  
7. **#4** Polyline lines — when bus count hurts  

---

## Acceptance (per item)

When implementing an item: tick Progress below, keep import constraints, run
`go test` under `examples/grid-sim-game`, append `CURSOR_HISTORY.md`.

### Progress

- [x] Leftovers (SpawnHouse comment, loadflow.md stub wording)
- [x] #5 Converge feedback in ImGui
- [x] #1 Procedural blank grid
- [ ] #2 Island / multi-slack — **removed** (reverted; keep simple global solve)
- [x] #3 Placement vs select split
- [x] #6 R+X line model
- [ ] #7 Save/load topology — **removed**
- [x] #4 Polyline line entity
- [ ] Optional E2 `network/powerflow` subpkg
