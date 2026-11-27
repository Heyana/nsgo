// Command nsgo develops and builds NSGo apps.
//
//	nsgo init  [-template go|tsx|mixed|module[@version][#name]] [-name N] [-module M] [-list] [dir]
//	                                                      write a new app
//	nsgo dev   [-target T] [-hot-reload=false] [-trace] [-automation] [-limits[=L]] [-preview PKG] [flags] [dir]
//	                                                      run the app and rebuild it on changes
//	nsgo mcp   [dev flags] [dir]                         serve MCP on standard input and output to drive the
//	                                                      app nsgo dev -automation runs, started if none does
//	nsgo build [-target all|T,...] [flags] [dir]         build packages for nsgo.yaml's targets or all platforms
//	nsgo test  [-run R] [-bench R] [-junit F] [-trace] [-target T [-fps]] [packages... | dir | entry] [-- go test flags]
//	                                                      run the packages' or app's tests or benchmarks, or an
//	                                                      entry with the steps of standard input, headless or
//	                                                      on a platform
//	nsgo check [-target T] [-jobs N] [dir]               vet the app and type-check its TSX
//	nsgo i18n  [-source L] [dir]                         check catalogs and write their Go code
//	nsgo add    [-dry-run] [-y] name|@registry/name|module[@version] ...
//	                                                      go get libraries once their plan is shown and
//	                                                      confirmed, never moving NSGo off its releases;
//	                                                      -list [@registry] [words...] lists NSGo's index
//	                                                      and configured registries
//	nsgo remove [-dry-run] [-y] name|@registry/name|module ...
//	                                                      drop libraries the module no longer imports,
//	                                                      through a go mod tidy shown and confirmed first
//	nsgo version                                         print which nsgo runs
//	nsgo doctor                                          show which targets this machine builds and runs
//	nsgo keygen                                          print a key pair for updates or signed packages
//	nsgo certify -key P -apps A [-days N] [-out F]       certify a release key for apps
//	nsgo publish itch -target web,macos_arm64 [dir]
//	                                                      upload built packages to itch.io
//	nsgo publish -cert C [-entry E] [-min-runtime V] [-out F] [dir | package]
//	                                                      compile a TSX app into a signed package, or sign one again
//	nsgo publish ios|macos <file>
//	                                                      upload a signed IPA or PKG to App Store Connect
//	nsgo publish -store appstore|play|all [-listings DIR] [-dry-run] [-platform P] [-version V] [-track T] [dir]
//	                                                      validate and upload store listings from YAML and Markdown files
//	nsgo publish -target android -track TRACK [dir | bundle.aab]
//	                                                      upload an Android App Bundle and commit a Play track release
//	nsgo verify  [-root P,... [-revocations F,...]] package
//	                                                      check a package and print what it holds
//	nsgo revoke  -sequence N [-days N] [-keys IDs] [-packages H,...] -out F
//	                                                      sign a revocation list
//	nsgo X [args]                                        run an extension: the module's go.mod tool X with
//	                                                      go tool, else nsgo-X on PATH
//
// In a module whose go.mod pins nsgo as a tool at another version, nsgo first runs that one, go tool nsgo, with the same arguments; NSGO_NO_DELEGATE=1 turns that off.
//
// dir is the app's directory, holding nsgo.yaml and its main package; it
// defaults to the current directory, and may end in /..., as go's
// patterns do. check also takes a directory without nsgo.yaml, such as a
// library or a module's root, and vets every package under it. Headless,
// test takes Go package patterns relative to the working directory (a
// directory names its own package, /... its tree, none tests ./...); a
// single app directory (with nsgo.yaml), a directory in another Go
// module, a TSX entry, or a -target run takes that directory or entry.
// Flags replace the nsgo.yaml values they name; -h lists them.
//
// Put flags before the path or package patterns. Headless, test forwards go test flags after them, -run, -timeout, -bench and -trace there included; a target run accepts only -short, -count, -cpu, -parallel, -failfast and -fullpath there.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	if code, delegated := delegate(context.Background(), os.Args[1:]); delegated {
		os.Exit(code)
	}
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
		preview := fs.String("preview", "", "preview an importable Go component package from this `directory`")
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		if *preview != "" {
			err = devPreview(ctx, f, *preview, fs.Arg(0))
			break
		}
		var p *project
		if p, err = load(fs.Arg(0)); err == nil {
			f.apply(p)
			err = dev(ctx, p, f.options())
		}
	case "mcp":
		fs := flag.NewFlagSet("mcp", flag.ExitOnError)
		f := bindDevFlags(fs)
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		var p *project
		if p, err = load(fs.Arg(0)); err == nil {
			f.apply(p)
			err = serveMCP(ctx, p, args)
		}
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		name := fs.String("name", "", "the app's name (default: the directory's)")
		module := fs.String("module", "", "write a go.mod for this module path (default outside a module: the app's slug)")
		kind := fs.String("template", "go", "the app to write: go, tsx, mixed, or a module path with optional @version and #name")
		list := fs.Bool("list", false, "list available templates")
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		if *list {
			if fs.NArg() > 0 || *name != "" || *module != "" {
				err = errors.New("init: -list takes only -template")
				break
			}
			err = listInitTemplates(ctx, os.Stdout, *kind)
			break
		}
		dir := fs.Arg(0)
		if dir == "" {
			dir = "."
		}
		err = initApp(ctx, dir, *name, *module, *kind)
	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		f := bindBuildFlags(fs)
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		var p *project
		if p, err = load(fs.Arg(0)); err == nil {
			f.apply(p)
			err = buildAll(ctx, p)
		}
	case "test":
		o := testOptions{}
		fs := testFlagSet(&o)
		if err = parseCommand(fs, args, -1); err != nil {
			break
		}
		paths, forwarded := splitTestArgs(fs.Args())
		o.args = forwarded
		// A headless run takes Go package patterns relative to cwd, so a shard
		// of the suite names exactly the packages it tests; a -target run, an
		// app directory, another Go module's directory, or a TSX entry loads
		// that directory or entry whole.
		dir := ""
		switch {
		case len(paths) == 0:
		case len(paths) == 1 && (o.target != "" || testEntry(paths[0])):
			dir = paths[0]
		default:
			o.patterns = paths
		}
		var p *project
		if p, err = loadForTest(dir, os.Stdin); err == nil {
			err = runTests(ctx, p, o)
		}
	case "check":
		fs := flag.NewFlagSet("check", flag.ExitOnError)
		target := fs.String("target", "", "vet for these comma-separated `targets` instead of the host (all for available Go/Xcode targets; a bare OS such as windows vets both its architectures)")
		jobs := fs.Int("jobs", 0, "vet up to `n` targets at once, each go vet with n compile workers (default: one target at a time, with go's own default)")
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		var p *project
		if p, err = loadOrDir(treeOf(fs.Arg(0))); err == nil {
			err = check(ctx, p, *target, *jobs)
		}
	case "i18n":
		fs := flag.NewFlagSet("i18n", flag.ExitOnError)
		source := fs.String("source", "", "the `language` a library's catalogs are written in (default: as their code says, else en)")
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		dir := fs.Arg(0)
		if dir == "" {
			dir = "."
		}
		if _, statErr := os.Stat(filepath.Join(dir, "nsgo.yaml")); statErr == nil {
			var p *project
			if p, err = load(dir); err == nil {
				err = p.prepareCatalogs(ctx, false, ".")
			}
		} else {
			err = generateCatalog(ctx, dir, *source)
		}
	case "add":
		fs := flag.NewFlagSet("add", flag.ExitOnError)
		var o addOptions
		fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan and write nothing")
		fs.BoolVar(&o.yes, "y", false, "apply the plan without asking")
		fs.BoolVar(&o.list, "list", false, "list libraries and components from NSGo's index and configured registries, filtered by optional @registry or words")
		if err = fs.Parse(args); err != nil {
			break
		}
		var ask func(context.Context) (bool, error)
		if stdinIsTerminal() {
			ask = askToApply
		}
		err = addModules(ctx, o, fs.Args(), os.Stdout, ask)
		// An interrupt fails add, which says what it left of go.mod and go.sum, rather than ending quietly with status 0 as the other commands do: the status tells a script whether the files were written.
		if err != nil && ctx.Err() != nil {
			report(err)
			os.Exit(130)
		}
	case "remove":
		fs := flag.NewFlagSet("remove", flag.ExitOnError)
		var o removeOptions
		fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan and write nothing")
		fs.BoolVar(&o.yes, "y", false, "apply the plan without asking")
		if err = fs.Parse(args); err != nil {
			break
		}
		var ask func(context.Context) (bool, error)
		if stdinIsTerminal() {
			ask = askToApplyRemove
		}
		err = removeModules(ctx, o, fs.Args(), os.Stdout, ask)
		// An interrupt fails remove, which says what it left of go.mod and go.sum, rather than ending quietly with status 0 as the other commands do: the status tells a script whether the files were written.
		if err != nil && ctx.Err() != nil {
			report(err)
			os.Exit(130)
		}
	case "version":
		fs := flag.NewFlagSet("version", flag.ExitOnError)
		if err = parseCommand(fs, args, 0); err != nil {
			break
		}
		printVersion(ctx)
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		if err = parseCommand(fs, args, 0); err != nil {
			break
		}
		err = doctor(ctx)
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ExitOnError)
		if err = parseCommand(fs, args, 0); err != nil {
			break
		}
		err = keygen()
	case "certify":
		fs := flag.NewFlagSet("certify", flag.ExitOnError)
		var o certifyOptions
		fs.StringVar(&o.key, "key", "", "the release key's public `key`, as nsgo keygen prints it; the root's secret key is NSGO_ROOT_KEY's")
		fs.StringVar(&o.apps, "apps", "", "the comma-separated app `IDs` it may sign: *, com.example.* or an ID")
		fs.IntVar(&o.days, "days", 365, "how many `days` from now it may sign")
		fs.StringVar(&o.out, "out", "", "the certificate's `file` (default: release.cert)")
		if err = parseCommand(fs, args, 0); err != nil {
			break
		}
		err = certify(o)
	case "publish":
		if len(args) > 0 && args[0] == "itch" {
			err = publishItch(ctx, args[1:])
			break
		}
		if len(args) > 0 && (args[0] == "ios" || args[0] == "macos") {
			err = publishApple(ctx, args[0], args[1:])
			break
		}
		fs := flag.NewFlagSet("publish", flag.ExitOnError)
		var o publishOptions
		target := fs.String("target", "", "publish an Android App Bundle to Google Play with `android`")
		fs.StringVar(&o.cert, "cert", "", "the release key's certificate `file`; its secret key is NSGO_RELEASE_KEY's")
		fs.StringVar(&o.entry, "entry", "", "the TSX `entry` to compile (default: the one the main package runs, else App.tsx)")
		fs.StringVar(&o.minRuntime, "min-runtime", "", "the oldest runtime `version` the package runs on (default: this tool's)")
		fs.StringVar(&o.extra, "extra", "", "opaque metadata `file` to sign into the package")
		fs.StringVar(&o.out, "out", "", "the package's `file` (default: build.output's <name>-<version>.nsgopkg)")
		fs.StringVar(&o.store, "store", "", "upload store listings to comma-separated `stores`: appstore, play or all")
		fs.StringVar(&o.listings, "listings", "", "the store listings `directory` holding listing.yaml and locale subdirectories (default: nsgo.yaml's store.dir, else store)")
		fs.BoolVar(&o.dryRun, "dry-run", false, "parse and validate store listings without uploading")
		fs.StringVar(&o.platform, "platform", "", "the App Store Connect `platform`: ios, macos, tvos or visionos (default: listing.yaml's app_store.platform, else ios)")
		fs.StringVar(&o.version, "version", "", "the App Store Connect app `version` string (default: listing.yaml's app_store.version, else nsgo.yaml's version)")
		fs.StringVar(&o.track, "track", "", "the Google Play `track`: with -target android, the one to release the App Bundle to, such as internal, production or a closed track's ID; with -store, the one to update release notes on (default: listing.yaml's google_play.track)")
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		switch *target {
		case "android":
			rest := o
			rest.track = ""
			if rest != (publishOptions{}) {
				err = errors.New("publish: -target android takes only -track and an App Bundle or project directory")
				break
			}
			err = publishPlay(ctx, o.track, fs.Arg(0))
		case "":
			err = publishWithContext(ctx, o, fs.Arg(0))
		default:
			err = fmt.Errorf("publish: unknown target %q", *target)
		}
	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		var o verifyOptions
		fs.StringVar(&o.roots, "root", "", "the comma-separated public `keys` of the roots to trust (default: check the package against itself)")
		fs.StringVar(&o.revocations, "revocations", "", "the comma-separated revocation list `files`, a root's newest each, to check it against")
		if err = parseCommand(fs, args, 1); err != nil {
			break
		}
		if fs.NArg() != 1 {
			usage()
		}
		err = verifyPackage(o, fs.Arg(0))
	case "revoke":
		fs := flag.NewFlagSet("revoke", flag.ExitOnError)
		var o revokeOptions
		fs.Uint64Var(&o.sequence, "sequence", 0, "the list's `number`, above the root's last one's; the root's secret key is NSGO_ROOT_KEY's")
		fs.IntVar(&o.days, "days", 0, "how many `days` from now the list lasts, which hosts may require a newer one by (default: it never expires)")
		fs.StringVar(&o.keys, "keys", "", "the comma-separated `IDs` of the release keys to revoke")
		fs.StringVar(&o.packages, "packages", "", "the comma-separated packages to revoke, by `SHA-256` or file")
		fs.StringVar(&o.out, "out", "", "the list's `file`")
		if err = parseCommand(fs, args, 0); err != nil {
			break
		}
		err = revoke(o)
	default:
		var code int
		var found bool
		if code, found, err = runExtension(ctx, cmd, args); found && err == nil {
			os.Exit(code)
		}
		if err == nil {
			usage()
		}
	}
	if err != nil && ctx.Err() == nil {
		report(err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  nsgo init  [-template go|tsx|mixed|module[@version][#name]] [-name N] [-module M] [-list] [dir]
  nsgo dev   [-target native|web|ios|ios_simulator|tvos|tvos_simulator|android] [-android-host] [-hot-reload=false] [-release] [-trace] [-profile cpu|heap|trace] [-automation] [-fps] [-lang L] [-device D] [-port N] [-open=false] [dir]
  nsgo mcp   [dev flags] [dir]
  nsgo build [-target all|macos_arm64,windows_x64,linux_x64,ios,web,...] [-output DIR] [-open] [-fps] [-run [-device D]] [dir]
  nsgo test  [-run REGEXP] [-bench REGEXP] [-junit FILE] [-trace] [-target macos|windows|linux|web|ios_simulator|tvos_simulator|android [-fps]] [packages... | dir | App.tsx < steps] [-- go test flags]
  nsgo check [-target all|macos_arm64,windows_x64,linux_x64,ios,web,...] [dir]
  nsgo i18n  [-source L] [dir]
  nsgo add    [-dry-run] [-y] name|@registry/name|module[@version] ...
  nsgo add    -list [@registry] [words...]
  nsgo remove [-dry-run] [-y] name|@registry/name|module ...
  nsgo doctor
  nsgo version
  nsgo keygen
  nsgo certify -key P -apps A [-days N] [-out F]
  nsgo publish itch -target web,macos_arm64 [dir]
  nsgo publish -cert C [-entry E] [-min-runtime V] [-extra F] [-out F] [dir | package]
  nsgo publish -store appstore|play|all [-listings DIR] [-dry-run] [-platform P] [-version V] [-track T] [dir]
  nsgo publish ios|macos <file>
  nsgo publish -target android -track TRACK [dir | bundle.aab]
  nsgo verify  [-root P,... [-revocations F,...]] package
  nsgo revoke  -sequence N [-days N] [-keys IDs] [-packages H,...] -out F

dev and build take -name, -id and -version, which replace nsgo.yaml's
values, and -h lists every flag. Put NSGo flags before the path or
package patterns. After --, headless tests accept go test flags; target
tests accept -short, -count, -cpu, -parallel, -failfast and -fullpath.

add is go get after a plan: it shows what go.mod and go.sum would
get, refuses to move NSGo off its releases, and asks before writing;
remove refuses libraries the module still imports and plans go mod tidy
on copies of go.mod and go.sum without lowering another module. In both,
-y applies without asking, and without a terminal nothing is written
unless -y is given. A module that pins an nsgo from before add or remove
is told which release to get instead.

Any other command X runs the module's go.mod tool X with go tool, else
nsgo-X on PATH. In a module that pins nsgo at another version, its own
nsgo runs instead.`)
	if found := listExtensions(context.Background()); len(found) > 0 {
		width := 0
		for _, ext := range found {
			width = max(width, len(ext.Name))
		}
		fmt.Fprintln(os.Stderr, "\nextensions:")
		for _, ext := range found {
			fmt.Fprintf(os.Stderr, "  nsgo %-*s  %s\n", width, ext.Name, ext.source())
		}
	}
	os.Exit(2)
}
