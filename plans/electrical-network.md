# Electrical Network — Design Sketch

## Overview

A separate `ElectricalNetwork` resource that lives alongside the ECS world but
owns its own graph (buses + branches). Grid entities are joined to the network
via `NetworkLink` component and the network's internal `entityBus` map.

---

## Types (`entities/network.go`)

```go
// BusType distinguishes how a bus participates in power flow.
type BusType int
const (
    BusGenerator BusType = iota // slack / PV — source
    BusLoad                     // PQ — sink (house)
    BusJunction                 // passive node (line segment)
)

type BusID    uint64
type BranchID uint64

// Bus is a node in the electrical network.
type Bus struct {
    ID     BusID
    Type   BusType
    Entity ecs.Entity // back-link to the grid entity
}

// Branch is an edge between two buses. Parameters (impedance etc.) go here later.
type Branch struct {
    ID   BranchID
    From BusID
    To   BusID
}

// NetworkLink is an ECS component — the grid-side half of the join.
// Every entity that has a bus carries this so ECS filters can reach it.
type NetworkLink struct{ BusID BusID }

// ElectricalNetwork is the per-World resource. It owns the graph.
type ElectricalNetwork struct {
    buses      map[BusID]*Bus
    branches   map[BranchID]*Branch
    entityBus  map[ecs.Entity]BusID   // join: entity → bus
    incidence  map[BusID][]BranchID   // adjacency list
    nextBus    BusID
    nextBranch BranchID
}
```

---

## Interface

```go
func NewElectricalNetwork() *ElectricalNetwork

// Join interface — called from PlacementSystem
func (n *ElectricalNetwork) AddBus(e ecs.Entity, t BusType) *Bus
func (n *ElectricalNetwork) BusForEntity(e ecs.Entity) (*Bus, bool)
func (n *ElectricalNetwork) RemoveBus(id BusID)   // also drops incident branches

// Graph operations
func (n *ElectricalNetwork) AddBranch(from, to BusID) *Branch
func (n *ElectricalNetwork) RemoveBranch(id BranchID)
func (n *ElectricalNetwork) Neighbors(id BusID) []*Bus

// Read access for simulation systems
func (n *ElectricalNetwork) Buses()    map[BusID]*Bus
func (n *ElectricalNetwork) Branches() map[BranchID]*Branch
```

---

## Wiring into placement (`entities/placement.go`)

Called after every successful spawn. Uses `GridOccupancy` to find the 4
cardinal neighbours and adds a branch to any that already have a bus — so the
graph stays in sync with the grid automatically, O(1) per placement.

```go
func attachToNetwork(w *ecs.World, e ecs.Entity, kind Tool, cell GridCoord, occ *GridOccupancy) {
    net := ecs.GetResource[ElectricalNetwork](w)
    bus := net.AddBus(e, toolToBusType(kind))
    ecs.NewMap1[NetworkLink](w).Add(e, &NetworkLink{BusID: bus.ID})

    for _, nb := range cardinalNeighbours(cell) {
        if ne, ok := occ.Cells[nb]; ok {
            if nbBus, ok := net.BusForEntity(ne); ok {
                net.AddBranch(bus.ID, nbBus.ID)   // bidirectional
            }
        }
    }
}
```

---

## Registration (`states/grid_state.go`)

One extra line in `Enter`:

```go
ecs.SetResource(s.World(), entities.NewElectricalNetwork())
```

---

## What this enables

A future `PowerFlowSystem` can call `ecs.GetResource[ElectricalNetwork](w)` and
operate entirely on `Buses()` / `Branches()` — no ECS scanning required, the
graph is self-contained.

Possible extensions to `Bus` / `Branch`:

| Field | Where | Purpose |
|---|---|---|
| `VoltageMag`, `VoltageAng` | `Bus` | Nodal voltage (power flow result) |
| `PInject`, `QInject` | `Bus` | Active / reactive power injection |
| `Resistance`, `Reactance` | `Branch` | Line impedance (Z = R + jX) |
| `MaxCurrent` | `Branch` | Thermal limit |

---

## Relationship to ECS

```
GridOccupancy          ElectricalNetwork
  Cells: coord→entity    buses:     BusID→Bus
                         branches:  BranchID→Branch
                         entityBus: Entity→BusID  ←── join
                         Bus.Entity              ←── reverse join

ECS entity
  GridObject   (grid position + kind)
  NetworkLink  (BusID)              ←── ECS-side handle
  Sprite, Position, ...
```

Blank tiles are **not** registered in the network — only generators, houses,
and line segments.
