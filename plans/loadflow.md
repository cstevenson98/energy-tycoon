# Load Flow & Network Analysis Architecture

## Layers

```
ElectricalNetwork  (topology + Dirty flag + StaticState)
│
├── StaticState    (solved snapshot: bus voltages, branch flows, LastError)
│   ├── map[BusID]*BusState
│   └── map[BranchID]*BranchState
│
├── LoadflowSystem (sole caller of Solve; Dirty → solve → ClearDirty)
│
└── LoadflowSolver (AC Newton–Raphson)
    ├── BuildYBus → SparseMatrix G + jB
    └── pkg/nr (+ SuperLU when CGo available)
```

History rings (`BusHistory` / `BranchHistory`) record samples after each
successful or partially-run solve; see `history.go`.

---

## State types (`network/state.go`)

### BusSpec — boundary conditions (entity → solver)

What the grid entity (or a system reading its ECS components) provides
before solving. Meaningful fields depend on `BusFormulation` (not `BusType`):

| Formulation | Fixed fields          | Free fields       |
|-------------|-----------------------|-------------------|
| Slack       | VoltMag, VoltAng      | PInject, QInject  |
| PV          | VoltMag, PInject      | VoltAng, QInject  |
| PQ          | PInject, QInject      | VoltMag, VoltAng  |

Generators default to Slack at `NominalVoltageV`; houses are PQ loads set by
`systems/wiring` from `HouseLoad`; line tiles are Junction (PQ=0).

```go
type BusSpec struct {
    Formulation BusFormulation
    VoltMag     float64  // volts — Slack + PV
    VoltAng     float64  // radians — Slack only
    PInject     float64  // W (+gen) — PV + PQ
    QInject     float64  // VAR — PQ only
}
```

### BusResult / BranchResult — solver output

Written by `LoadflowSolver.Solve` into `StaticState`. Branch flows are
positive in the from→to direction; `|I|` in amps.

### StaticState — the snapshot container

```go
type StaticState struct {
    Buses      map[BusID]*BusState
    Branches   map[BranchID]*BranchState
    Converged  bool
    Iterations int
    LastError  string // empty on success; shown in ImGui
}
```

Entries are created/destroyed automatically in `AddBus` / `RemoveBus` /
`AddBranch` / `RemoveBranch` — StaticState always mirrors the topology.

---

## Pipeline (Dirty → solve)

1. Placement / wiring / load-tick mutate graph or specs → `MarkDirty()`.
2. `LoadflowSystem.Update` sees Dirty, calls `LoadflowSolver.Solve`.
3. Solver: require ≥1 slack, build Y-bus (`y = 1/(r+jx)`), run NR via `pkg/nr`.
4. Results + `LastError` land in `StaticState`; history may append; Dirty cleared.

Sign convention: `P_spec > 0` is generation. House loads use
`PQSpec(−P_kW*1000, −Q_kVAR*1000)`.

---

## Entity → Solver API

```go
net.SetBusSpec(busID, SlackSpec(230, 0))
net.SetBusSpec(busID, PQSpec(-2100, -1800))
bs, _ := net.BusStateFor(busID)
```

ImGui reads `net.State` (converged, iterations, `LastError`, per-bus results).

---

## Solver interface (`network/solver.go`)

```go
type Solver interface {
    Solve(net *ElectricalNetwork) error
}
```

`LoadflowSolver` is the production AC NR implementation (not a stub). Tunables:
`MaxIter` (default 50), `Tol` (default 1e-6 on ‖f‖₂).

---

## Time evolution

Not implemented as a dedicated stepper. House demand re-samples via
`LoadTickSystem`; history rings cover recent solve samples. A TimeEvolution
container can be added later if schedules need explicit snapshots beyond
Dirty-driven solves.

---

## File layout

```
game/components/network/
  network.go   ← topology graph + State + entity-join API
  state.go     ← BusSpec, BusResult, StaticState, formulations
  solver.go    ← Solver interface, LoadflowSolver (AC NR)
  history.go   ← Series / BusHistory / BranchHistory
  ybus.go      ← Y-bus build + CalcPQ
  sparse.go    ← SparseMatrix
  doc.go       ← package glossary

pkg/nr/        ← Newton–Raphson + SuperLU (CGo) / dense fallback
```
