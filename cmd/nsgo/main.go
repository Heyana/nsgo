// Command nsgo develops and builds NSGo apps.
//
//	nsgo init  [-name N] [-module M] [dir]               write a new app
//	nsgo dev   [-target T] [-hot-reload] [flags] [dir]   run the app and rebuild it on changes
//	nsgo build [-target all|T,...] [flags] [dir]         build packages for nsgo.yaml's targets or all platforms
//
// dir is the app's directory, holding nsgo.yaml and its main package; it
// defaults to the current directory. Flags replace the nsgo.yaml values
// they name; -h lists them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
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
		f := bindDevFlags(fs)
		fs.Parse(args)
		var p *project
		if p, err = load(fs.Arg(0)); err == nil {
			f.apply(p)
			err = dev(ctx, p, devOptions{apply: f.apply, port: f.port, open: f.open, device: f.device, team: f.team})
		}
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		name := fs.String("name", "", "the app's name (default: the directory's)")
		module := fs.String("module", "", "write a go.mod for this module path")
		fs.Parse(args)
		dir := fs.Arg(0)
		if dir == "" {
			dir = "."
		}
		err = initApp(dir, *name, *module)
	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		f := bindBuildFlags(fs)
		fs.Parse(args)
		var p *project
		if p, err = load(fs.Arg(0)); err == nil {
			f.apply(p)
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
  nsgo init  [-name N] [-module M] [dir]
  nsgo dev   [-target native|web|ios|ios_simulator|android] [-hot-reload] [-port N] [-open=false] [dir]
  nsgo build [-target all|macos_arm64,windows_x64,linux_x64,ios,web,...] [-output DIR] [-open=false] [dir]

dev and build take -name, -id, -version, -width and -height, which replace
nsgo.yaml's values, and -h lists every flag.`)
	os.Exit(2)
}
