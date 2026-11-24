// The fixture isolates packaging from the UI engine. Its small Go payload records shell arguments and toast hooks, and can stay running for the installer's owned-process shutdown check. It never changes a default handler.
package main

import (
	_ "dev.nsgo.windowsinstallerfixture/locales"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	dir := filepath.Dir(executable)
	version, _ := os.ReadFile(filepath.Join(dir, "version.txt"))
	record := struct {
		PID     int
		Version string
		Args    []string
	}{os.Getpid(), string(version), os.Args[1:]}
	data, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	file, err := os.OpenFile(filepath.Join(dir, "fixture-calls.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	_, err = file.Write(append(data, '\n'))
	file.Close()
	if err != nil {
		panic(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "--stay" {
		for {
			time.Sleep(time.Second)
		}
	}
}
