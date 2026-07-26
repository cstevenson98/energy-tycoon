# Package Refactor Plan

## Current layout

```
game/
  main.go
  gameconfig/
    gameconfig.go
  entities/                   ← monolithic: components + systems mixed
    components.go
    spawn.go
    placement.go
    camera_scroll.go
    toolbar.go
states/
  grid_state.go
assets/
  art/
  fonts/
```

## Target layout

```
game/
  main.go
  gameconfig/
    gameconfig.go
  components/
    grid/                     package grid
      grid.go                   Tool, GridCoord, GridObject, GridOccupancy, PlacementState
      spawn.go                  SpawnBlank, SpawnGenerator, SpawnHouse, SpawnLineSegment, ManhattanPath
      toolbar.go                ToolbarButton, ToolbarButtons
    network/                  package network
      network.go                BusType, BusID, BranchID, Bus, Branch, NetworkLink, ElectricalNetwork
  systems/
    placement/                package placement
      placement.go              PlacementSystem
    camera/                   package camera
      camera.go                 CameraScrollSystem
states/
  grid_state.go
assets/
  art/
  fonts/
plans/
```

## Import graph (no cycles)

```
gameconfig        ← no game imports
components/grid   ← gameconfig, pkg/ecs, pkg/components
components/network← pkg/ecs
systems/placement ← components/grid, components/network, gameconfig, pkg/ecs, pkg/components
systems/camera    ← gameconfig, pkg/ecs, pkg/components
states            ← components/grid, components/network, systems/placement, systems/camera, gameconfig, pkg/...
game/main.go      ← states, pkg/engine, pkg/config, pkg/types, pkg/logger
```

## Migration steps

1. Create `game/components/grid/` — move `entities/components.go` contents minus
   `NetworkLink` (that goes in `network/`). Move `spawn.go` and `toolbar.go` unchanged.
   Update package declaration to `package grid`.

2. Create `game/components/network/` — new; see `electrical-network.md` for design.
   `NetworkLink` component lives here alongside `ElectricalNetwork`.

3. Create `game/systems/placement/` — move `entities/placement.go`. Update imports
   from `entities.Foo` → `grid.Foo` / `network.Foo`.

4. Create `game/systems/camera/` — move `entities/camera_scroll.go`. Update imports.

5. Update `states/grid_state.go` imports: replace `entities` with `grid`, `network`,
   `placement`, `camera` packages as appropriate.

6. Delete `game/entities/`.

7. `go mod tidy` in `examples/grid-sim-game/`.

## Notes

- Spawn helpers stay in `components/grid` rather than `systems/placement` because
  they describe how to construct a grid entity (data concern), not how to respond to
  input (behaviour concern). `PlacementSystem` calls them but doesn't own them.
- `ToolbarButtons()` stays in `components/grid` (shared by both the placement system
  for hit-testing and `grid_state` for rendering — neither owns it exclusively).
- The `gameconfig` package is unchanged; all packages import it as a leaf.
