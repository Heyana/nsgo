### Changed

- Loop music players on Linux and the web without a gap at each repeat, decoding tracks up to 32 MiB and 5 minutes into memory (about 23 MB a minute) and, on Linux, streaming longer ones; a looping player on the web now plays only after a user gesture on the page and fails with `NotAllowed` before it (#1221 by @houko).
