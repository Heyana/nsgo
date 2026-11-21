package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type processOutput uint8

const (
	captureOutput processOutput = iota
	captureCombined
	streamOutput
)

// processCommand describes a command that runs to completion. The runner
// returns its output and leaves the caller to interpret failures.
type processCommand struct {
	name   string
	args   []string
	dir    string
	env    []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	output processOutput
}

type processRunner func(context.Context, processCommand) ([]byte, error)
type processRunnerKey struct{}

func execute(ctx context.Context, c processCommand) ([]byte, error) {
	if runner, ok := ctx.Value(processRunnerKey{}).(processRunner); ok {
		return runner(ctx, c)
	}
	cmd := exec.CommandContext(ctx, c.name, c.args...)
	cmd.Dir, cmd.Env, cmd.Stdin = c.dir, c.env, c.stdin
	cmd.Stdout, cmd.Stderr = c.stdout, c.stderr
	switch c.output {
	case captureCombined:
		return cmd.CombinedOutput()
	case streamOutput:
		return nil, cmd.Run()
	default:
		return cmd.Output()
	}
}

func run(ctx context.Context, name string, args ...string) error {
	// Tools' chatter only matters when they fail.
	if out, err := execute(ctx, processCommand{name: name, args: args, output: captureCombined}); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return nil
}

func output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := execute(ctx, processCommand{name: name, args: args})
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// goTool runs go with args in dir, its output shown only on a failure.
func goTool(ctx context.Context, dir string, env []string, args ...string) error {
	c := processCommand{
		name: "go", args: args, dir: dir, env: append(os.Environ(), env...), output: captureCombined,
	}
	if out, err := execute(ctx, c); err != nil {
		return fmt.Errorf("go %s failed\n%s", args[0], strings.TrimSpace(string(out)))
	}
	return nil
}
