### Changed

- Store canvas image sources and atlas sprites in reused `Drawing` buffers and pool drawings across redraws so steady-state `DrawImage*` and `DrawAtlas` frames allocate nothing in the driver or core (#2269).
