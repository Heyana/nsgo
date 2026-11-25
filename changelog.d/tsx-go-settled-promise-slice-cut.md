### Fixed

- Fail a `tsx.Plugin.Call` with `ErrBudget` when `Limits.Call` cuts a slice resumed by a Go-settled promise such as `host.call` (#2390).
