# Energy Tycoon

Grid / power-network sim game built on [`gowasm-engine`](https://github.com/cstevenson98/gowasm-engine).

## Local engine (dev)

This module depends on the engine via a local `replace` while the library is unpublished or you're hacking both side-by-side:

```go
replace github.com/cstevenson98/gowasm-engine => ../gowasm-engine
```

Expected layout:

```
~/dev/gowasm-engine/
~/dev/energy-tycoon/
```

## Dev shell (Nix)

Provides Go, Ebiten/GLFW/X11/GL, SuperLU (CGO), and sets `CGO_*` flags:

```bash
cd ~/dev/energy-tycoon
nix-shell          # or: direnv allow  (uses .envrc → use nix)
go run ./game
go test ./...
```

## Git LFS

Binary/game assets (images, audio, video, fonts, 3D, archives, …) are tracked with [Git LFS](https://git-lfs.com) via `.gitattributes`. `nix-shell` includes `git-lfs` and runs `git lfs install --local`.

```bash
git lfs install   # once per machine if not using nix-shell
git lfs ls-files  # see what's tracked
```

## Module

```
github.com/cstevenson98/energy-tycoon
```
