### Added

- Read `expo-secure-store` encrypted `SharedPreferences` entries (AES-GCM and legacy hybrid RSA) on Android, claim `keychain-access-groups` from `macos.entitlements` in Developer ID macOS builds, and store secrets in WeChat mini programs' platform-encrypted storage (`encrypt: true`) while keeping `ErrSecretsUnavailable` on the web and other mini programs (#1581).
