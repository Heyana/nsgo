### Fixed

- Measure `MaxLines`-truncated text boxes accurately on Windows (applying DirectWrite's height trim before reading width and keeping offered width for soft-wrapped cuts), mini programs (truncating the remaining text of the last paragraph for both `Measure` and `Layout`, including multi-line head/middle cuts) and HarmonyOS (applying `MaxLines` in Go over the host's `measure` bridge) (#1549).
