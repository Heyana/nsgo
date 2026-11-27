### Added

- Custom `registries:` in `nsgo.yaml` (`@name` to an HTTPS URL or relative YAML index) so `nsgo add @registry/name[@version]` resolves team libraries with ETag caching and offline fallback, and `nsgo add -list [@registry] [words…]` lists configured registry items (#2464).
