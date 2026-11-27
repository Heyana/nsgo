### Fixed

- Import bionic's `__errno` instead of glibc's `__errno_location` in Android arm64's cgo-free purego loader, lock the OS thread around `dlopen`/`dlsym`/`dlclose` and `dlerror`, validate the Android `libghostty-vt.so` prebuild during APK/AAB/AAR packaging, and clarify the missing-library error when an Android arm64 app is built without the `nsgo` tool (#1452).
