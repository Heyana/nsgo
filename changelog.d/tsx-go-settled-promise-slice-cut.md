### Fixed

- Fail a `tsx.Plugin.Call` with `ErrBudget` when `Limits.Call` cuts a slice resumed by a Go-settled promise such as `host.call`, and report the cut to `Sandbox.OnError` when the promise has no waiting call (#2390 by @owocc).
