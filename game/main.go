// Command game is the entry point for Energy Tycoon. Desktop: `go run ./game`.
// Browser: GOOS=js GOARCH=wasm go build -o main.wasm ./game (see engine examples Makefile patterns).
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/cstevenson98/energy-tycoon/states/grid"
	"github.com/cstevenson98/energy-tycoon/states/menu"
	"github.com/cstevenson98/milo/pkg/config"
	"github.com/cstevenson98/milo/pkg/engine"
	"github.com/cstevenson98/milo/pkg/logger"
	"github.com/cstevenson98/milo/pkg/types"
)

func main() {
	logger.Logger.Info("Grid sim game starting")

	// 1080p 16:9 virtual resolution. The world grid is much larger than the
	// viewport (see gameconfig.GridCols/Rows); explore with WASD / middle-mouse
	// pan. The right half of the window is the ImGui network panel.
	cfg := config.Default()
	cfg.Screen.Width = 1920
	cfg.Screen.Height = 1080
	cfg.Rendering.PixelScale = 1
	// Opt into ImGui for the right-half network inspector panel.
	gameEngine := engine.NewEngine(cfg).EnableImGui()

	gameEngine.RegisterState(types.MENU, menustate.NewMenuState())
	gameEngine.RegisterState(types.GAMEPLAY, gridstate.NewGridState())

	if err := gameEngine.Initialize("grid-canvas"); err != nil {
		log.Fatalf("Engine initialization failed: %s", err.Error())
	}

	if err := gameEngine.SetGameState(types.MENU); err != nil {
		log.Fatalf("Failed to set initial game state: %s", err.Error())
	}

	gameEngine.Start()

	ebiten.SetWindowSize(cfg.WindowWidth(), cfg.WindowHeight())
	ebiten.SetWindowTitle("Grid Sim Game")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if cfg.Rendering.PixelArtMode {
		ebiten.SetScreenFilterEnabled(false)
	}

	if err := ebiten.RunGame(gameEngine); err != nil {
		log.Fatal(err)
	}

	logger.Logger.Info("Grid sim game ended")
}
