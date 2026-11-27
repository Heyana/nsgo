package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

// Linux packages are written in Go, as no Mac has dpkg or makepkg: a
// Debian package, <slug>-<version>-linux-<arch>.deb, and an Arch Linux
// one, <slug>-<version>-linux-<arch>.pkg.tar.gz, named as the release's
// other files are, since dpkg and pacman read the package's name and
// version from inside it. Both install the package's
// directory in /opt/<slug>, a link to the app in /usr/bin and one to each
// of its commands, its desktop entry, its AppStream metadata, its icon in
// the hicolor theme and the files of linux.share under /usr/share.

// linuxPackageEntry is a file, directory or link a Linux package
// installs, by its path from the root.
type linuxPackageEntry struct {
	name string
	data []byte
	mode int64
	link string
	dir  bool
}

// validateLinuxDeps rejects a blank entry of depends or recommends, which would leave an empty item in a Debian list or an empty Arch line, and one holding a line break, which would add fields to the packages' metadata, naming it under field: linux for nsgo.yaml's, linux.<target> for an nsgo-natives.yaml's.
func validateLinuxDeps(field string, depends, recommends linuxDeps) error {
	return projectcfg.ValidateLinuxDeps(field, depends, recommends)
}

// linuxRequirements are the entries of the Debian and Arch packages' dependency fields for arch: NSGo's own, GTK 4 and, recommended as only web views load it, WebKitGTK; then the app's linux.depends and linux.recommends; then those that the packages its build links declare for the target in nsgo-natives.yaml (addPackageNatives), leaving out any entry the field holds already and any recommended one that a depend names, since a package one linked package needs is no longer optional because another only recommends it.
func (p *project) linuxRequirements(arch string) (depends, recommends linuxDeps) {
	depends = linuxDeps{Deb: append([]string{"libgtk-4-1"}, p.Linux.Depends.Deb...), Arch: append([]string{"gtk4"}, p.Linux.Depends.Arch...)}
	recommends = linuxDeps{Deb: append([]string{"libwebkitgtk-6.0-4"}, p.Linux.Recommends.Deb...), Arch: append([]string{"webkitgtk-6.0: web views"}, p.Linux.Recommends.Arch...)}
	linked := p.nativeLinuxDeps["linux_"+goArchName(arch)]
	for _, deps := range linked {
		addLinuxDeps(&depends, deps.Depends, linuxDeps{})
	}
	for _, deps := range linked {
		addLinuxDeps(&recommends, deps.Recommends, depends)
	}
	return depends, recommends
}

// addLinuxDeps appends each entry of extra that neither deps nor skip holds yet. An Arch entry is compared without the ": description" an optdepend carries, so libpulse in skip leaves out "libpulse: sound".
func addLinuxDeps(deps *linuxDeps, extra, skip linuxDeps) {
	for _, entry := range extra.Deb {
		if !slices.Contains(deps.Deb, entry) && !slices.Contains(skip.Deb, entry) {
			deps.Deb = append(deps.Deb, entry)
		}
	}
	for _, entry := range extra.Arch {
		name, _, _ := strings.Cut(entry, ":")
		if !slices.Contains(deps.Arch, entry) && !slices.Contains(skip.Arch, strings.TrimSpace(name)) {
			deps.Arch = append(deps.Arch, entry)
		}
	}
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
	// The packages own the links, so removing them removes the links too.
	for _, path := range p.commandFiles(dir, "linux") {
		name := filepath.Base(path)
		files = append(files, linuxPackageEntry{name: "usr/bin/" + name, link: "/" + opt + "/" + name, mode: 0o777})
	}

	share, err := p.linuxShareFiles()
	if err != nil {
		return nil, err
	}
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
	if err := p.addLinuxShare(share, files, func(e linuxPackageEntry) { add(e.name, e.data, e.mode) }); err != nil {
		return nil, err
	}
	return files, nil
}

// linuxShareFiles are the files of linux.share, by their paths in the
// system's usr/share, each regular file executable or not as it is there.
func (p *project) linuxShareFiles() ([]linuxPackageEntry, error) {
	if p.Linux.Share == "" {
		return nil, nil
	}
	info, err := os.Stat(p.Linux.Share)
	if err != nil {
		return nil, fmt.Errorf("linux.share: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("linux.share: %s is not a directory", rel(p.Linux.Share))
	}
	var files []linuxPackageEntry
	err = filepath.WalkDir(p.Linux.Share, func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("linux.share: %s is not a regular file", rel(file))
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		mode := int64(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		name, _ := filepath.Rel(p.Linux.Share, file)
		files = append(files, linuxPackageEntry{name: "usr/share/" + filepath.ToSlash(name), data: data, mode: mode})
		return nil
	})
	return files, err
}

// addLinuxShare adds the AppStream metadata, unless share or files hold the app's own, and share's files, with what NSGo adds there such as the AppImage's link to its desktop entry, to a package whose entries so far are files, through add. A file of share at a path the package holds already, such as the desktop entry or an icon NSGo writes, or at a directory it needs, such as applications, is refused.
func (p *project) addLinuxShare(share, files []linuxPackageEntry, add func(linuxPackageEntry)) error {
	metainfo := "usr/share/metainfo/" + p.ID + ".metainfo.xml"
	named := func(name string) func(linuxPackageEntry) bool {
		return func(e linuxPackageEntry) bool { return e.name == name && !e.dir }
	}
	if !slices.ContainsFunc(share, named(metainfo)) && !slices.ContainsFunc(files, named(metainfo)) {
		data, err := p.metainfo(true)
		if err != nil {
			return err
		}
		share = append([]linuxPackageEntry{{name: metainfo, data: data, mode: 0o644}}, share...)
	}
	// dirs records each path the package holds, true for a directory, so a file where a directory is needed collides in either order.
	dirs := map[string]bool{}
	hold := func(name string, dir bool) {
		dirs[name] = dir
		for d := path.Dir(name); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	for _, f := range files {
		hold(f.name, f.dir)
	}
	for _, e := range share {
		_, taken := dirs[e.name]
		for d := path.Dir(e.name); d != "." && !taken; d = path.Dir(d) {
			dir, held := dirs[d]
			taken = held && !dir
		}
		if taken {
			return fmt.Errorf("%s, from linux.share or NSGo, collides with a file the package holds already", e.name)
		}
		hold(e.name, false)
		add(e)
	}
	return nil
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
	depends, recommends := p.linuxRequirements(arch)
	control := fmt.Sprintf("Package: %s\nVersion: %s\nArchitecture: %s\nMaintainer: %s\nInstalled-Size: %d\n"+
		"Depends: %s\nRecommends: %s\nSection: misc\nPriority: optional\nDescription: %s\n",
		p.packageSlug(), debVersion(p.Version), debArch[arch], p.maintainer(), (linuxPackageSize(files)+1023)/1024,
		strings.Join(depends.Deb, ", "), strings.Join(recommends.Deb, ", "), name)
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
	now := time.Unix(time.Now().Unix(), 0)
	var info strings.Builder
	fmt.Fprintf(&info, "pkgname = %s\npkgbase = %s\npkgver = %s-1\npkgdesc = %s\nbuilddate = %d\npackager = %s\nsize = %d\narch = %s\n",
		slug, slug, archVersion(p.Version), name, now.Unix(), p.maintainer(), linuxPackageSize(files), archLinuxArch[arch])
	depends, recommends := p.linuxRequirements(arch)
	for _, dep := range depends.Arch {
		fmt.Fprintf(&info, "depend = %s\n", dep)
	}
	for _, dep := range recommends.Arch {
		fmt.Fprintf(&info, "optdepend = %s\n", dep)
	}
	entries := append([]linuxPackageEntry{{name: ".PKGINFO", data: []byte(info.String()), mode: 0o644}}, files...)
	if p.linuxRegisters() {
		entries = append(entries, linuxPackageEntry{name: ".INSTALL", data: []byte(archCacheScript), mode: 0o644})
	}
	mtree, err := archMtree(entries, now)
	if err != nil {
		return nil, err
	}
	entries = append(entries, linuxPackageEntry{name: ".MTREE", data: mtree, mode: 0o644})
	return tarGzAt(entries, "", now)
}

// archMtree writes ALPM-MTREEv2 (https://man.archlinux.org/man/ALPM-MTREEv2.5.en), gzip-compressed as pacman expects. It describes the metadata and payload, excluding itself, with the same timestamp as their tar headers.
func archMtree(entries []linuxPackageEntry, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := fmt.Fprintln(gz, "#mtree"); err != nil {
		return nil, err
	}
	for _, e := range entries {
		kind := "file"
		if e.dir {
			kind = "dir"
		} else if e.link != "" {
			kind = "link"
		}
		if _, err := fmt.Fprintf(gz, "%s type=%s uid=0 gid=0 mode=%o time=%d.0", mtreeEscape("./"+e.name), kind, e.mode, now.Unix()); err != nil {
			return nil, err
		}
		switch kind {
		case "file":
			if _, err := fmt.Fprintf(gz, " size=%d sha256digest=%x", len(e.data), sha256.Sum256(e.data)); err != nil {
				return nil, err
			}
		case "link":
			if _, err := fmt.Fprintf(gz, " link=%s", mtreeEscape(e.link)); err != nil {
				return nil, err
			}
		}
		if _, err := fmt.Fprintln(gz); err != nil {
			return nil, err
		}
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// mtreeEscape keeps paths and link targets to one token, encoding their bytes as mtree's three-digit octal escapes.
func mtreeEscape(s string) string {
	var out strings.Builder
	for i := range len(s) {
		b := s[i]
		if b <= ' ' || b >= 0x7f || b == '\\' || b == '#' {
			fmt.Fprintf(&out, "\\%03o", b)
		} else {
			out.WriteByte(b)
		}
	}
	return out.String()
}

// tarGz is a gzip-compressed tar archive of entries owned by root, each
// name after prefix.
func tarGz(entries []linuxPackageEntry, prefix string) ([]byte, error) {
	return tarGzAt(entries, prefix, time.Now())
}

func tarGzAt(entries []linuxPackageEntry, prefix string, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		// dpkg rejects PAX extended headers, which Go otherwise selects for Unicode names. GNU headers also support long paths and link targets.
		hdr := &tar.Header{Format: tar.FormatGNU, Name: prefix + e.name, Mode: e.mode, ModTime: now, Uname: "root", Gname: "root",
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
