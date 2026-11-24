### Changed

- Build the Windows setup and its signed uninstaller with Go alone, with native Windows dialogs, so `build` no longer needs NSIS's `makensis` or leaves a `.nsi` script; installations, file types, URL schemes and `Path` entries of the earlier NSIS installers upgrade in place (#1408 by @houko).
