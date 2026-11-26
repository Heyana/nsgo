### Added

- Ship a package's iOS and tvOS frameworks through `nsgo-natives.yaml` version 2, which `nsgo build`, `nsgo dev` and `-run` check against the app, sign with it and credit in its notices, and `-embed` refuses for now (#2473 by @houko).
- Put the libraries that a terminal target's packages declare for its desktop beside the executable (#2473 by @houko).
