package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Linux packages are written in Go, as no Mac has dpkg or makepkg: a
// Debian package, <slug>-<version>-linux-<arch>.deb, and an Arch Linux
// one, <slug>-<version>-linux-<arch>.pkg.tar.gz, named as the release's
// other files are, since dpkg and pacman read the package's name and
// version from inside it. Both install the package's
// directory in /opt/<slug>, a link to the app in /usr/bin, its desktop
// entry and its icon in the hicolor theme.

// linuxPackageEntry is a file, directory or link a Linux package
// installs, by its path from the root.
type linuxPackageEntry struct {
	name string
	data []byte
	mode int64
	link string
	dir  bool
}

// linuxDeps lists packages for each Linux package format, each entry as that format writes it: Debian's alternatives and version constraints ("a | b", "foo (>= 1.2)") or Arch's "name: reason" optional dependencies.
type linuxDeps struct {
	Deb  []string `yaml:"deb"`
	Arch []string `yaml:"arch"`
}

// validateLinuxDeps rejects a blank entry, which would leave an empty item in a Debian list or an empty Arch line, and one holding a line break, which would add fields to the packages' metadata.
func (p *project) validateLinuxDeps() error {
	for _, key := range []struct {
		name   string
		values []string
	}{
		{"depends.deb", p.Linux.Depends.Deb},
		{"depends.arch", p.Linux.Depends.Arch},
		{"recommends.deb", p.Linux.Recommends.Deb},
		{"recommends.arch", p.Linux.Recommends.Arch},
	} {
		for i, value := range key.values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("linux.%s entry %d is empty", key.name, i+1)
			}
			if strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("linux.%s entry %d contains a line break", key.name, i+1)
			}
		}
	}
	return nil
}

// linuxMainCategories are the freedesktop Desktop Menu Specification's main categories, by which menus file an app; an entry naming none of them lands under Other.
var linuxMainCategories = []string{"AudioVideo", "Audio", "Video", "Development", "Education", "Game", "Graphics", "Network", "Office", "Science", "Settings", "System", "Utility"}

// linuxAdditionalCategories are the specification's additional categories, which refine a main one.
var linuxAdditionalCategories = []string{
	"Building", "Debugger", "IDE", "GUIDesigner", "Profiling", "RevisionControl", "Translation",
	"Calendar", "ContactManagement", "Database", "Dictionary", "Chart", "Email", "Finance", "FlowChart", "PDA", "ProjectManagement", "Presentation", "Spreadsheet", "WordProcessor",
	"2DGraphics", "VectorGraphics", "RasterGraphics", "3DGraphics", "Scanning", "OCR", "Photography", "Publishing", "Viewer",
	"TextTools", "DesktopSettings", "HardwareSettings", "Printing", "PackageManager",
	"Dialup", "InstantMessaging", "Chat", "IRCClient", "Feed", "FileTransfer", "HamRadio", "News", "P2P", "RemoteAccess", "Telephony", "TelephonyTools", "VideoConference", "WebBrowser", "WebDevelopment",
	"Midi", "Mixer", "Sequencer", "Tuner", "TV", "AudioVideoEditing", "Player", "Recorder", "DiscBurning",
	"ActionGame", "AdventureGame", "ArcadeGame", "BoardGame", "BlocksGame", "CardGame", "KidsGame", "LogicGame", "RolePlaying", "Shooter", "Simulation", "SportsGame", "StrategyGame",
	"Art", "Construction", "Music", "Languages", "ArtificialIntelligence", "Astronomy", "Biology", "Chemistry", "ComputerScience", "DataVisualization", "Economy", "Electricity", "Geography", "Geology", "Geoscience", "History", "Humanities", "ImageProcessing", "Literature", "Maps", "Math", "NumericalAnalysis", "MedicalSoftware", "Physics", "Robotics", "Spirituality", "Sports", "ParallelComputing",
	"Amusement", "Archiving", "Compression", "Electronics", "Emulator", "Engineering", "FileTools", "FileManager", "TerminalEmulator", "Filesystem", "Monitor", "Security", "Accessibility", "Calculator", "Clock", "TextEditor", "Documentation", "Adult",
	"Core", "KDE", "GNOME", "XFCE", "DDE", "LXQt", "GTK", "Qt", "Motif", "Java", "ConsoleOnly",
}

// linuxReservedCategories are for desktop components, and desktop-file-validate accepts them only beside an OnlyShowIn key, which an app's entry does not have.
var linuxReservedCategories = []string{"Screensaver", "TrayIcon", "Applet", "Shell"}

// validateLinuxCategories accepts the desktop entry's categories that desktop-file-validate does: each registered, or an extension starting with X- of letters, digits, '-', '_' and '.', once each, with AudioVideo beside Audio or Video. It also asks for a main category, without which menus file the app under Other, the case the setting exists to avoid.
func (p *project) validateLinuxCategories() error {
	categories := p.linuxCategories()
	if len(categories) == 0 {
		return nil
	}
	main := false
	for i, c := range categories {
		switch {
		case slices.Contains(linuxMainCategories, c):
			main = true
		case slices.Contains(linuxAdditionalCategories, c):
		case slices.Contains(linuxReservedCategories, c):
			return fmt.Errorf("linux.categories entry %d, %q, is reserved for desktop components", i+1, c)
		case strings.HasPrefix(c, "X-") && len(c) > 2 && strings.Trim(c[2:], "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.") == "":
		default:
			return fmt.Errorf("linux.categories entry %d, %q, is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'", i+1, c)
		}
		if slices.Index(categories, c) < i {
			return fmt.Errorf("linux.categories names %q twice", c)
		}
	}
	for _, c := range []string{"Audio", "Video"} {
		if slices.Contains(categories, c) && !slices.Contains(categories, "AudioVideo") {
			return fmt.Errorf("linux.categories names %s without AudioVideo, which it belongs to", c)
		}
	}
	if !main {
		return fmt.Errorf("linux.categories names no main category, so menus would file the app under Other; add one of %s", strings.Join(linuxMainCategories, ", "))
	}
	return nil
}

// linuxDependency is a Debian dependency field: base, then extra.
func linuxDependency(base string, extra []string) string {
	if len(extra) == 0 {
		return base
	}
	return strings.Join(append([]string{base}, extra...), ", ")
}

// buildLinuxPackages writes the Debian and Arch packages of the Linux
// package in dir, built for arch, beside it, and returns their paths.
func buildLinuxPackages(ctx context.Context, p *project, dir, arch string) (string, string, error) {
	stage(ctx, "writing the Linux packages")
	files, err := linuxPackageFiles(p, dir)
	if err != nil {
		return "", "", err
	}
	deb, err := debPackage(p, files, arch)
	if err != nil {
		return "", "", err
	}
	debPath := filepath.Join(dir, debName(p, arch))
	if err := os.WriteFile(debPath, deb, 0o644); err != nil {
		return "", "", err
	}
	pkg, err := archPackage(p, files, arch)
	if err != nil {
		return "", "", err
	}
	pkgPath := filepath.Join(dir, archPackageName(p, arch))
	return debPath, pkgPath, os.WriteFile(pkgPath, pkg, 0o644)
}

// linuxPackageFiles lists what the packages install from the Linux
// package in dir, in the order they create it, with no directory twice.
func linuxPackageFiles(p *project, dir string) ([]linuxPackageEntry, error) {
	slug := p.packageSlug()
	opt := "opt/" + slug
	var files []linuxPackageEntry
	dirs := map[string]bool{}
	mkdir := func(name string) {
		var parts []string
		for _, part := range strings.Split(name, "/") {
			parts = append(parts, part)
			if d := strings.Join(parts, "/"); !dirs[d] {
				dirs[d] = true
				files = append(files, linuxPackageEntry{name: d, mode: 0o755, dir: true})
			}
		}
	}
	add := func(name string, data []byte, mode int64) {
		mkdir(path.Dir(name))
		files = append(files, linuxPackageEntry{name: name, data: data, mode: mode})
	}
	mkdir(opt)
	err := filepath.WalkDir(dir, func(file string, d fs.DirEntry, err error) error {
		if err != nil || file == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, file)
		rel = filepath.ToSlash(rel)
		// What the packages put where desktops look for it, and the
		// packages themselves, stay out of /opt.
		if !d.IsDir() && !strings.Contains(rel, "/") && (rel == p.ID+".desktop" || rel == p.ID+".png" || rel == p.ID+".mime.xml" || isLinuxPackage(rel)) {
			return nil
		}
		name := opt + "/" + rel
		if d.IsDir() {
			mkdir(name)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(file)
			if err != nil {
				return err
			}
			files = append(files, linuxPackageEntry{name: name, link: link, mode: 0o777})
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		mode := int64(0o644)
		if rel == p.Name || info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		add(name, data, mode)
		return nil
	})
	if err != nil {
		return nil, err
	}
	mkdir("usr/bin")
	files = append(files, linuxPackageEntry{name: "usr/bin/" + slug, link: "/" + opt + "/" + p.Name, mode: 0o777})

	n, err := p.appNames()
	if err != nil {
		return nil, err
	}
	types, definitions, err := p.linuxFileTypes()
	if err != nil {
		return nil, err
	}
	entry := desktopData{Name: p.Name, Exec: "/" + opt + "/" + p.Name, ID: p.ID, MIMETypes: types, URLSchemes: p.URLSchemes, Categories: p.linuxCategories()}
	if n != nil {
		entry.Name = n.names[n.source]
		for _, lang := range n.languages()[1:] {
			entry.Names = append(entry.Names, [2]string{desktopLocale(lang), n.names[lang]})
		}
	}
	var desktop bytes.Buffer
	if err := desktopEntry.Execute(&desktop, entry); err != nil {
		return nil, err
	}
	add("usr/share/applications/"+p.ID+".desktop", desktop.Bytes(), 0o644)
	if len(definitions) > 0 {
		add("usr/share/mime/packages/"+p.ID+".xml", definitions, 0o644)
	}
	img, err := p.linuxIconImage()
	if err != nil {
		return nil, err
	}
	if img != nil {
		for _, size := range []int{256, 512} {
			add(fmt.Sprintf("usr/share/icons/hicolor/%dx%d/apps/%s.png", size, size, p.ID), scaledPNG(img, size), 0o644)
		}
	}
	return files, nil
}

// linuxPackageSize is how many bytes the files take installed.
func linuxPackageSize(files []linuxPackageEntry) int64 {
	var size int64
	for _, f := range files {
		size += int64(len(f.data))
	}
	return size
}

// maintainer is who the Linux packages name as theirs.
func (p *project) maintainer() string {
	if p.Linux.Maintainer != "" {
		return p.Linux.Maintainer
	}
	return p.Name
}

// debArch and archLinuxArch are the Debian and Arch Linux names of Go's
// architectures.
var (
	debArch       = map[string]string{"amd64": "amd64", "arm64": "arm64"}
	archLinuxArch = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}
)

// debVersion is a version as Debian orders it: a pre-release, 1.2.0-beta.1,
// sorts before its release as 1.2.0~beta.1.
func debVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	if base, pre, ok := strings.Cut(v, "-"); ok {
		v = base + "~" + strings.ReplaceAll(pre, "-", ".")
	}
	return strings.ReplaceAll(v, "+", "~")
}

// archVersion is a version as pacman takes it, which allows no hyphen: a
// pre-release keeps its words after an underscore.
func archVersion(v string) string {
	return strings.NewReplacer("-", "_", "+", "_").Replace(strings.TrimPrefix(v, "v"))
}

func debName(p *project, arch string) string {
	return p.releaseName("linux_"+goArchName(arch), ".deb")
}

func archPackageName(p *project, arch string) string {
	return p.releaseName("linux_"+goArchName(arch), ".pkg.tar.gz")
}

// debPackage is the Debian package installing files: an ar archive of
// debian-binary, control.tar.gz and data.tar.gz. It depends on GTK 4 and
// recommends WebKitGTK, which only web views load.
func debPackage(p *project, files []linuxPackageEntry, arch string) ([]byte, error) {
	var md5sums bytes.Buffer
	for _, f := range files {
		if !f.dir && f.link == "" {
			fmt.Fprintf(&md5sums, "%x  %s\n", md5.Sum(f.data), f.name)
		}
	}
	name, err := p.displayName()
	if err != nil {
		return nil, err
	}
	control := fmt.Sprintf("Package: %s\nVersion: %s\nArchitecture: %s\nMaintainer: %s\nInstalled-Size: %d\n"+
		"Depends: %s\nRecommends: %s\nSection: misc\nPriority: optional\nDescription: %s\n",
		p.packageSlug(), debVersion(p.Version), debArch[arch], p.maintainer(), (linuxPackageSize(files)+1023)/1024,
		linuxDependency("libgtk-4-1", p.Linux.Depends.Deb), linuxDependency("libwebkitgtk-6.0-4", p.Linux.Recommends.Deb), name)
	controlFiles := []linuxPackageEntry{
		{name: "control", data: []byte(control), mode: 0o644},
		{name: "md5sums", data: md5sums.Bytes(), mode: 0o644},
	}
	if p.linuxRegisters() {
		for _, script := range []string{"postinst", "postrm"} {
			controlFiles = append(controlFiles, linuxPackageEntry{name: script, data: []byte(linuxCacheScript), mode: 0o755})
		}
	}
	controlTar, err := tarGz(controlFiles, "./")
	if err != nil {
		return nil, err
	}
	dataTar, err := tarGz(files, "./")
	if err != nil {
		return nil, err
	}
	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	now := time.Now().Unix()
	for _, m := range []struct {
		name string
		data []byte
	}{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", controlTar}, {"data.tar.gz", dataTar}} {
		fmt.Fprintf(&deb, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", m.name, now, 0, 0, "100644", len(m.data))
		deb.Write(m.data)
		if len(m.data)%2 == 1 {
			deb.WriteByte('\n')
		}
	}
	return deb.Bytes(), nil
}

// archPackage is the Arch Linux package installing files: a tar archive
// whose .PKGINFO, first, describes it. It depends on gtk4 and offers
// webkitgtk-6.0 for web views.
func archPackage(p *project, files []linuxPackageEntry, arch string) ([]byte, error) {
	name, err := p.displayName()
	if err != nil {
		return nil, err
	}
	slug := p.packageSlug()
	var info strings.Builder
	fmt.Fprintf(&info, "pkgname = %s\npkgbase = %s\npkgver = %s-1\npkgdesc = %s\nbuilddate = %d\npackager = %s\nsize = %d\narch = %s\n",
		slug, slug, archVersion(p.Version), name, time.Now().Unix(), p.maintainer(), linuxPackageSize(files), archLinuxArch[arch])
	info.WriteString("depend = gtk4\n")
	for _, dep := range p.Linux.Depends.Arch {
		fmt.Fprintf(&info, "depend = %s\n", dep)
	}
	info.WriteString("optdepend = webkitgtk-6.0: web views\n")
	for _, dep := range p.Linux.Recommends.Arch {
		fmt.Fprintf(&info, "optdepend = %s\n", dep)
	}
	entries := append([]linuxPackageEntry{{name: ".PKGINFO", data: []byte(info.String()), mode: 0o644}}, files...)
	if p.linuxRegisters() {
		entries = append(entries, linuxPackageEntry{name: ".INSTALL", data: []byte(archCacheScript), mode: 0o644})
	}
	return tarGz(entries, "")
}

// tarGz is a gzip-compressed tar archive of entries owned by root, each
// name after prefix.
func tarGz(entries []linuxPackageEntry, prefix string) ([]byte, error) {
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	now := time.Now()
	for _, e := range entries {
		hdr := &tar.Header{Name: prefix + e.name, Mode: e.mode, ModTime: now, Uname: "root", Gname: "root",
			Typeflag: tar.TypeReg, Size: int64(len(e.data))}
		switch {
		case e.dir:
			hdr.Typeflag, hdr.Name, hdr.Size = tar.TypeDir, hdr.Name+"/", 0
		case e.link != "":
			hdr.Typeflag, hdr.Linkname, hdr.Size = tar.TypeSymlink, e.link, 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if hdr.Size > 0 {
			if _, err := tw.Write(e.data); err != nil {
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
