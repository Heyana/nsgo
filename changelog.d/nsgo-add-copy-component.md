### Added

- Copy declared components into the app with `nsgo add [-diff] [-dir D] [-tests=false] module[@version]#component`, verifying module zips, enforcing copy rules and license policy, rewriting imports and comments, generating catalogs, vetting targets, recording metadata in `nsgo-copy.yaml`, and placing copied notices under their own `THIRD_PARTY_NOTICES.txt` heading (#2467).
