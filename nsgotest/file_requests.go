package nsgotest

import (
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ImagePick records the app's photo picker or camera options.
type ImagePick struct {
	Limit        int
	MaxDimension int
	Quality      float32
	Format       core.PickFormat
}

func imagePick(r driver.PickRequest) ImagePick {
	return ImagePick{Limit: r.Limit, MaxDimension: r.MaxDimension, Quality: r.Quality, Format: r.Format}
}

// CodeScan records the kinds of codes the app asked to scan. An empty
// list asks for QR codes.
type CodeScan struct {
	Kinds []core.CodeKind
}

// Wants reports whether the scanner reads the given kind.
func (s CodeScan) Wants(k core.CodeKind) bool {
	return (driver.ScanRequest{Kinds: s.Kinds}).Wants(k)
}

func codeScan(r driver.ScanRequest) CodeScan {
	return CodeScan{Kinds: slices.Clone(r.Kinds)}
}

// FilePick records the app's options for picking files.
type FilePick struct {
	Limit     int
	Title     string
	Button    string
	TypesName string
	Start     string
	Types     []string
}

func filePick(r driver.FilesRequest) FilePick {
	return FilePick{
		Limit: r.Limit, Title: r.Title, Button: r.Button, TypesName: r.TypesName,
		Start: r.Start, Types: slices.Clone(r.Types),
	}
}

// DirectoryPick records the app's options for picking directories.
type DirectoryPick struct {
	Limit  int
	Title  string
	Button string
	Start  string
}

func directoryPick(r driver.DirectoriesRequest) DirectoryPick {
	return DirectoryPick{Limit: r.Limit, Title: r.Title, Button: r.Button, Start: r.Start}
}

// FileSave records the app's suggested file name and what it wants to
// save, from the file at Path when given or from Data.
type FileSave struct {
	Name  string
	MIME  string
	Start string
	Path  string
	Data  []byte
	// Preflight asks the platform to confirm the destination before writing.
	Preflight bool
}

func fileSave(r driver.SaveRequest) FileSave {
	return FileSave{Name: r.Name, MIME: r.MIME, Start: r.Start, Path: r.Path, Data: slices.Clone(r.Data), Preflight: r.Preflight}
}
