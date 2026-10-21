// Command nsgo develops and builds NSGo apps.
//
//	nsgo dev   [-target T] [-port N] [dir]   run the app and rebuild it on changes
//	nsgo build [-target T,...] [dir]         build packages for nsgo.yaml's targets
//
// dir is the app's directory, holding nsgo.yaml and its main package; it
// defaults to the current directory.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "dev":
		fs := flag.NewFlagSet("dev", flag.ExitOnError)
		target := fs.String("target", "", "native, web or ios_simulator (default: nsgo.yaml's dev.target)")
		port := fs.Int("port", 8080, "the web target's port")
		open := fs.Bool("open", true, "open the web target in the browser")
		fs.Parse(args)
		var p *project
		if p, err = load(fs.Arg(0)); err == nil {
			err = dev(ctx, p, devOptions{target: *target, port: *port, open: *open})
		}
	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		targets := fs.String("target", "", "comma-separated targets (default: nsgo.yaml's build.targets)")
		fs.Parse(args)
		var p *project
		if p, err = load(fs.Arg(0)); err == nil {
			if *targets != "" {
				p.Build.Targets = strings.Split(*targets, ",")
			}
			err = buildAll(ctx, p)
		}
	default:
		usage()
	}
	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "nsgo:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  nsgo dev   [-target native|web|ios_simulator] [-port N] [-open=false] [dir]
  nsgo build [-target macos_arm64,windows_x64,ios,web,...] [dir]`)
	os.Exit(2)
}
