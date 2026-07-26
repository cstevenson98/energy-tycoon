// Package network is the electrical domain for grid-sim-game.
//
// # Layers
//
//   - Graph: ElectricalNetwork, Bus, Branch, NetworkLink (network.go)
//   - Specs / results: BusSpec, BusFormulation, StaticState (state.go)
//   - History rings: Series, BusHistory, BranchHistory (history.go)
//   - Load-flow math: YBus, SparseMatrix, LoadflowSolver (ybus.go, sparse.go, solver.go)
//
// # Glossary
//
//   - BusType — role of a node on the grid (Generator, Load, Junction).
//   - BusFormulation — solver boundary condition (Slack, PV, PQ).
//
// Generators default to Slack at NominalVoltageV; houses are PQ loads set by
// systems/wiring from HouseLoad; line tiles are Junction buses with segment R.
//
// This package must not import game/components/grid (join is via NetworkLink
// and entity handles). LoadflowSystem is the sole caller of LoadflowSolver.Solve.
package network
