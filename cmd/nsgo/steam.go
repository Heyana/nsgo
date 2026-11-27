package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	steamAppIDFile = "steam_appid.txt"
	steamBuildTag  = "nsgo_steam"
)

func (p *project) checkSteamTargets(targets []string) error {
	if p.channel() != channelSteam {
		return nil
	}
	if p.buildEmbed {
		return errors.New("-channel steam makes desktop depots and cannot be combined with -embed")
	}
	for _, target := range targets {
		if !desktopTarget(target) {
			return fmt.Errorf("-channel steam builds macOS, Windows and Linux depots: remove %s from the targets", target)
		}
	}
	return nil
}

func (p *project) steamDevelopment() bool {
	return p.devClient && p.Dev.Target == "native" && p.Steam.AppID != 0
}

func (p *project) steamBuild(target string) bool {
	return desktopTarget(target) && (p.channel() == channelSteam || p.steamDevelopment())
}

func startDesktopDev(p *project, opts devOptions, dir, bin string) (app, error) {
	workDir := ""
	if p.steamDevelopment() {
		// Steamworks reads the file from the working directory. On macOS
		// this keeps it beside the signed .app bundle.
		id := strconv.FormatUint(uint64(p.Steam.AppID), 10)
		if err := os.WriteFile(filepath.Join(dir, steamAppIDFile), []byte(id), 0o644); err != nil {
			return app{}, err
		}
		workDir = dir
	}
	if opts.debugger != nil {
		return opts.debugger.start(workDir, dir, devEnv(opts), bin, opts.appFlags())
	}
	stop, wait, err := startProcessIn(workDir, devEnv(opts), bin, opts.appFlags()...)
	if err != nil {
		return app{}, err
	}
	return app{stop: stop, wait: wait}, nil
}
