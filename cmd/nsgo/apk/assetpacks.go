package apk

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

const distNS = "http://schemas.android.com/apk/distribution"

// AssetPack is a Play Asset Delivery module. Files in a pack are read
// from disk while signing and archiving the bundle.
type AssetPack struct {
	Name, Delivery string
	Files          []AssetPackFile
}

// AssetPackFile is a relative asset path and the local file that supplies it.
type AssetPackFile struct {
	Name, Source string
}

var assetPackName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidAssetPackName reports whether name can identify a Play asset module.
func ValidAssetPackName(name string) bool { return name != "base" && assetPackName.MatchString(name) }

type bundleEntry struct {
	File
	source string
}

func bundleAssetEntries(appID string, packs []AssetPack) ([]bundleEntry, error) {
	var entries []bundleEntry
	names := map[string]bool{}
	for _, pack := range packs {
		if !ValidAssetPackName(pack.Name) || names[pack.Name] {
			return nil, fmt.Errorf("invalid or duplicate asset pack name %q", pack.Name)
		}
		names[pack.Name] = true
		if pack.Delivery != "install-time" && pack.Delivery != "fast-follow" {
			return nil, fmt.Errorf("asset pack %s has invalid delivery %q", pack.Name, pack.Delivery)
		}
		root := &node{name: xml.Name{Local: "manifest"}, attrs: []xml.Attr{
			{Name: xml.Name{Local: "package"}, Value: appID},
			{Name: xml.Name{Local: "split"}, Value: pack.Name},
			{Name: xml.Name{Space: "xmlns", Local: "dist"}, Value: distNS},
		}}
		module := &node{name: xml.Name{Space: distNS, Local: "module"}, attrs: []xml.Attr{
			{Name: xml.Name{Space: distNS, Local: "type"}, Value: "asset-pack"},
		}}
		include := "true"
		if pack.Delivery == "fast-follow" {
			include = "false"
		}
		module.kids = []*node{
			{name: xml.Name{Space: distNS, Local: "delivery"}, kids: []*node{{name: xml.Name{Space: distNS, Local: pack.Delivery}}}},
			{name: xml.Name{Space: distNS, Local: "fusing"}, attrs: []xml.Attr{{Name: xml.Name{Space: distNS, Local: "include"}, Value: include}}},
		}
		root.kids = []*node{module}
		manifest, err := protoElement(root, true, false)
		if err != nil {
			return nil, err
		}
		entries = append(entries, bundleEntry{File: File{Name: pack.Name + "/manifest/AndroidManifest.xml", Data: pbBytes(nil, 1, manifest)}})
		prefix := pack.Name + "/assets/nsgo_asset_packs/" + pack.Name + "/"
		entries = append(entries, bundleEntry{File: File{Name: prefix + "_nsgo_pack", Data: []byte(pack.Delivery)}})
		files := map[string]bool{"_nsgo_pack": true}
		for _, f := range pack.Files {
			if f.Source == "" || f.Name == "." || !fs.ValidPath(f.Name) || strings.ContainsAny(f.Name, "\\\r\n") || files[f.Name] {
				return nil, fmt.Errorf("asset pack %s has invalid or duplicate file %q", pack.Name, f.Name)
			}
			files[f.Name] = true
			entries = append(entries, bundleEntry{File: File{Name: prefix + f.Name}, source: f.Source})
		}
	}
	return entries, nil
}

func (e bundleEntry) copyTo(w io.Writer) error {
	if e.source == "" {
		_, err := io.Copy(w, bytes.NewReader(e.Data))
		return err
	}
	f, err := os.Open(e.source)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func writeBundleArchive(w io.Writer, files []File, packs []bundleEntry, key *Key) error {
	entries := make([]bundleEntry, 0, len(files)+len(packs))
	for _, f := range files {
		entries = append(entries, bundleEntry{File: f})
	}
	entries = append(entries, packs...)
	digests := make([]jarDigest, 0, len(entries))
	for _, e := range entries {
		hash := sha256.New()
		if err := e.copyTo(hash); err != nil {
			return err
		}
		digests = append(digests, jarDigest{e.Name, [sha256.Size]byte(hash.Sum(nil))})
	}
	signature, err := signJar(digests, key)
	if err != nil {
		return err
	}
	z := zip.NewWriter(w)
	add := func(f File) (io.Writer, error) {
		method := uint16(zip.Deflate)
		if f.Stored {
			method = zip.Store
		}
		return z.CreateHeader(&zip.FileHeader{Name: f.Name, Method: method, ModifiedDate: dosDate})
	}
	for _, f := range signature {
		dst, err := add(f)
		if err != nil {
			return err
		}
		if _, err := dst.Write(f.Data); err != nil {
			return err
		}
	}
	for i, e := range entries {
		dst, err := add(e.File)
		if err != nil {
			return err
		}
		hash := sha256.New()
		if err := e.copyTo(io.MultiWriter(dst, hash)); err != nil {
			return err
		}
		if !bytes.Equal(hash.Sum(nil), digests[i].sum[:]) {
			return errors.New("a bundle asset changed during packaging: " + e.Name)
		}
	}
	return z.Close()
}
