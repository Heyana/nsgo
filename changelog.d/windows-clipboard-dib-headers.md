### Fixed

- Read V4/V5 `CF_DIB` clipboard bitmaps with bitfield masks inside the header and reject truncated `BI_BITFIELDS` or non-BGRA alpha masks on Windows (#2213 by @Heyana).
