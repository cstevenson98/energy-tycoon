# Easy-wins refactor plan

Goal: land all assessment wins for digestibility/maintainability **without**
reshaping the import graph or churning `pkg/nr`.

Estimated total: ~1–2 focused sessions. Prefer one PR (or 2–3 stacked) so
tests stay green at each step.

## Constraints (do not violate)

- Keep **acyclic** imports: `gameconfig` → `grid` / `network` → systems → `states`
- **`network` must not import `grid`**
- Keep **Dirty → `LoadflowSystem` sole solver caller**
- Leave `pkg/nr` alone except if a compile break forces a touch
- No behaviour change unless noted (load range comment fix is intentional)

## Execution order

Do in this order so later steps sit on a cleaner base.

| Phase | Wins | Risk |
|-------|------|------|
| A | 4, 5 (docs + dead code + AddBus) | Low |
| B | 1, 7 (split UI + Tool labels) | Low |
| C | 2, 6, 8 (wiring extract + logging) | Medium |
| D | 3, 9 (tests) | Low |
| E | 10 (network domain clarity) | Medium |

---

## Phase A — Tiny cleanups

### A1. Fix `RandLoadKW` (`spawn.go`) — win #4

**Decision (pick one before coding):** keep current physics `[1.5, 3]` kW and
fix the comment (recommended — matches LV cable tune), **or** restore
`[10, 20]` if gameplay still wants exaggerated loads.

- Update comment + any ImGui/docs that still say 10–20
- Optional: named constants `HouseLoadMinKW` / `HouseLoadMaxKW`

### A2. Remove `TimeEvolution` (`solver.go`) — win #5a

- Delete type + methods (~`solver.go` 405–425)
- Update `plans/loadflow.md` “Future: TimeEvolution” to stay aspirational
  (or note “not implemented”)

### A3. `AddBus` returns `(*Bus, error)` — win #5b

```go
func (n *ElectricalNetwork) AddBus(e ecs.Entity, t BusType) (*Bus, error)
```

- Error on duplicate entity (replace panic)
- Update all call sites: placement wiring, all `network_*_test.go` helpers
- Drop `TestAddBusDuplicatePanics` → `TestAddBusDuplicateError`
- Callers log-and-skip (placement) rather than panic

---

## Phase B — UI hub split + labels

### B1. Split `states/grid_state.go` — win #1

Same package `states`. Suggested file cut:

| File | Contents |
|------|----------|
| `grid_state.go` | `GridState`, `NewGridState`, `Enter`, `DrawOverlays`, `RenderImGui` (thin orchestrators) |
| `grid_overlays.go` | `renderToolbar`, `renderGridChrome`, `drawCellBorder`, `drawPlacementGhost`, `ghostColors` |
| `grid_imgui.go` | `renderNetworkPanel`, `renderSelectionPanel`, history helpers (`collectBusHistories`, charts) |

No API / behaviour change. Verify desktop still draws toolbar + ImGui.

### B2. Consolidate Tool display strings — win #7

On `grid.Tool` (in `grid.go`):

```go
func (t Tool) Label() string        // already exists (toolbar)
func (t Tool) GhostLetter() string  // "G"/"H"/"L"/"X"
func (t Tool) KindLabel() string    // "Generator"/"House"/… for inspector
```

On `network.BusFormulation`:

```go
func (f BusFormulation) String() string  // replaces formulationLabel
```

Delete `toolGhostLetter`, `toolKindLabel`, `formulationLabel` from states.

---

## Phase C — Wiring extract + solve logging

### C1. Extract network attach/delete — win #2

**New file:** `game/systems/wiring/wiring.go` (or `game/components/network/wiring.go`
only if it never imports `grid` — **prefer `systems/wiring`** because today
attach reads `HouseLoad` / `LineSegmentProps` / `GridOccupancy`).

Recommended API (keeps grid knowledge in the systems layer):

```go
package wiring

func Attach(w *ecs.World, e ecs.Entity, kind grid.Tool, cell grid.GridCoord, occ *grid.GridOccupancy) error
func Detach(w *ecs.World, e ecs.Entity) // RemoveBus if linked; caller owns occupancy/ECS remove
```

- Move `attachToNetwork`, `toolToBusType` from `placement.go`
- `deleteCell` stays in placement for occupancy + `w.Remove`, but calls
  `wiring.Detach` for the network half
- Placement imports `wiring`; `wiring` imports `grid` + `network` (still no cycle)
- Handle `AddBus` error from A3

### C2. Gate solve logging — win #8

In `loadflow.go`:

- Remove unconditional `net.Log()` / `net.LogVoltages()` from every dirty solve
- Option A (preferred): `logger.Logger.Debugf` only, or a `gameconfig.DebugLoadflowLog bool` default false
- Keep `Print`/`Log`/`LogVoltages` on `ElectricalNetwork` for manual debug, or
  move to `network/debug.go` as package-level helpers taking `*ElectricalNetwork`

No change to ImGui inspector (it already shows live state).

---

## Phase D — Tests

### D1. Grid geometry tests — win #3

New: `game/components/grid/coords_test.go`, `path_test.go` (or one `grid_test.go`)

| Case | Assert |
|------|--------|
| `ManhattanPath` same cell | `[]{from}` or empty — match current behaviour |
| L-shape / straight | Correct length and order |
| `ScreenToCell` with identity cam | Known pixel → cell |
| Out of bounds / toolbar band | `ok == false` where applicable |
| `CellScreenRect` | Inverse of `ScreenToCell` for cell centre |

Needs `gameconfig.Global` initialized in `TestMain` or per-test (same pattern
as other game tests if any; else set `Global` fields explicitly).

### D2. Pipeline smoke test — win #9

New: `game/systems/loadtick/loadtick_test.go` and/or
`game/systems/wiring/wiring_test.go`

Minimal World setup:

1. Set resources: occupancy, network, (optional placement)
2. Spawn gen + line + house; `wiring.Attach` each
3. Force Dirty; run `LoadflowSystem.Update` → Dirty cleared, `Converged` or logged
4. Run `LoadTickSystem` with `accum >= Interval` → house P/Q changed, Dirty set
5. Second loadflow clears Dirty again

Keep this as an integration-style test in `systems/`; do not pull ImGui/Ebiten.

---

## Phase E — Clarify `network` as domain package — win #10

**Do last.** Prefer the smallest clarity win that does not force mass import edits.

### Option E1 (recommended first): docs + internal layout

- Expand `network` package comment: “electrical domain: graph resource + AC
  loadflow; not pure ECS components”
- Split files only (already mostly split): ensure `solver.go` / `ybus.go` /
  `sparse.go` stay clearly “math”, `network.go` / `state.go` “graph”
- Optional: `network/doc.go` with a short glossary (`BusType` vs `BusFormulation`)

### Option E2 (if still muddy): subpackage move

```
game/components/network/     # graph, NetworkLink, ElectricalNetwork, state, history
game/components/network/lf/  # LoadflowSolver, YBus, SparseMatrix  (name TBD)
```

- `lf` imports parent `network` types — **cannot** put graph in child if parent
  needs solver types in same package today; use `network` + `network/powerflow`
  with solver living in child and `LoadflowSolver` re-exported, **or** accept
  import `powerflow → network` and systems import both
- Touch: `loadflow` system, all solver tests package path
- Skip unless E1 feels insufficient after Phases A–D

**Do not** rename the module path or invent `game/domain/` in this pass.

---

## Acceptance checklist

- [x] `go test ./...` under `examples/grid-sim-game` passes (cgo SuperLU env)
- [ ] Desktop run: place gen/line/house, voltages in ImGui, load tick still updates
- [x] No `network` → `grid` import
- [x] Solve path quiet by default (no full topology dump every Dirty)
- [x] `CURSOR_HISTORY.md` entry after implementation
- [x] This plan marked done / dated when complete (implemented 2026-07-25; E2 subpkg deferred)

## Out of scope

- Camera refactor, ImGui redesign, WASM/SuperLU work
- Changing LV R / cell length again
- Menu / multi-state architecture
- Splitting placement hover vs select into two systems (optional later)

## Suggested PR slicing

1. **PR1:** Phase A + D1 (cleanups + grid tests)  
2. **PR2:** Phase B (UI split + labels)  
3. **PR3:** Phase C + D2 (wiring + logging + pipeline test)  
4. **PR4 (optional):** Phase E2 only if needed  

Or one PR with commits matching phases A→E.
