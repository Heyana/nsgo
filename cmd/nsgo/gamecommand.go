package main

import (
	"cmp"
	"context"
	"os"
	"os/exec"
	"slices"
)

// gameTool is the game layer's command, nsgo-dev/game's cmd/game, which nsgo game runs without nsgo importing the game module.
const gameTool = "github.com/nsgo-dev/game/packages/game/cmd/game"

// gameVersion is the version of the game layer nsgo game runs outside a module that declares gameTool; NSGO_GAME_VERSION replaces it.
const gameVersion = "v0.1.0"

// gameCommand is how nsgo game runs the game layer's command with args: the working directory's module's tool, at the version its go.mod pins, else go run of gameTool at gameVersion, which works outside a module and before a project exists.
func gameCommand(ctx context.Context, args []string) (*exec.Cmd, error) {
	tools, err := moduleTools(ctx, ".")
	if err != nil {
		return nil, err
	}
	if slices.Contains(tools, gameTool) {
		return goToolCommand(gameTool, args), nil
	}
	version := cmp.Or(os.Getenv("NSGO_GAME_VERSION"), gameVersion)
	return extensionCommand("go", append([]string{"run", gameTool + "@" + version}, args...)...), nil
}

// runGame runs nsgo game with args and returns its exit status.
func runGame(ctx context.Context, args []string) (int, error) {
	cmd, err := gameCommand(ctx, args)
	if err != nil {
		return 0, err
	}
	return runChild(cmd)
}
