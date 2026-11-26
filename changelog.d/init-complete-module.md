### Changed

- `nsgo init` writes a complete module, requiring the running `nsgo` at its version, declaring it as a tool and filling `go.sum`, so `nsgo dev` runs right after it; in an existing module it adds only the `require` and `tool` lines `go.mod` lacks, and the error for flags after the directory names the command as it was called (#2420).
