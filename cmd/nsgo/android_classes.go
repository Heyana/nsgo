package main

import (
	"archive/zip"
	"cmp"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

// androidClassFiles adds one secondary DEX per input shard. ART loads classes2.dex and later shards alongside the runtime's classes.dex.
func androidClassFiles(p *project) ([]apk.File, error) {
	var files []apk.File
	for _, name := range p.Android.Classes {
		shards, err := readAndroidClasses(p.resolve(name))
		if err != nil {
			return nil, fmt.Errorf("android.classes %q: %w", name, err)
		}
		for _, data := range shards {
			files = append(files, apk.File{Name: fmt.Sprintf("classes%d.dex", len(files)+2), Data: data})
		}
	}
	return files, nil
}

func readAndroidClasses(path string) ([][]byte, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".dex":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := checkAndroidDex(data); err != nil {
			return nil, err
		}
		return [][]byte{data}, nil
	case ".jar":
		return readAndroidDexJar(path)
	default:
		return nil, fmt.Errorf("want a precompiled .dex or a .jar containing DEX files")
	}
}

func readAndroidDexJar(path string) ([][]byte, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	type shard struct {
		index int
		file  *zip.File
	}
	var entries []shard
	for _, file := range z.File {
		if index := androidDexIndex(file.Name); index != 0 && !file.FileInfo().IsDir() {
			entries = append(entries, shard{index, file})
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("JAR has no root classes.dex shards; compile Java or Kotlin bytecode with D8 --min-api %d first", androidAPI)
	}
	slices.SortFunc(entries, func(a, b shard) int { return cmp.Compare(a.index, b.index) })
	shards := make([][]byte, 0, len(entries))
	for i, entry := range entries {
		if i > 0 && entry.index == entries[i-1].index {
			return nil, fmt.Errorf("JAR has duplicate %s entries", entry.file.Name)
		}
		r, err := entry.file.Open()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.file.Name, err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err == nil {
			err = checkAndroidDex(data)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.file.Name, err)
		}
		shards = append(shards, data)
	}
	return shards, nil
}

// androidDexIndex recognizes the root names produced by D8 and read by ART.
func androidDexIndex(name string) int {
	if name == "classes.dex" {
		return 1
	}
	digits, ok := strings.CutPrefix(name, "classes")
	if !ok {
		return 0
	}
	digits, ok = strings.CutSuffix(digits, ".dex")
	if !ok {
		return 0
	}
	index, err := strconv.Atoi(digits)
	if err != nil || index < 2 || strconv.Itoa(index) != digits {
		return 0
	}
	return index
}

// checkAndroidDex checks the classic header and the versions supported by the APK's minimum API. ART verifies the instructions when it loads them.
func checkAndroidDex(data []byte) error {
	const headerSize = 0x70
	if len(data) < headerSize || string(data[:4]) != "dex\n" || data[7] != 0 {
		return fmt.Errorf("invalid DEX header")
	}
	switch string(data[4:7]) {
	case "035", "037", "038", "039":
	default:
		return fmt.Errorf("DEX version %q is unsupported on Android API %d; compile with D8 --min-api %d", data[4:7], androidAPI, androidAPI)
	}
	if uint64(binary.LittleEndian.Uint32(data[32:36])) != uint64(len(data)) ||
		binary.LittleEndian.Uint32(data[36:40]) != headerSize ||
		binary.LittleEndian.Uint32(data[40:44]) != 0x12345678 {
		return fmt.Errorf("invalid DEX size or byte order")
	}
	return nil
}
