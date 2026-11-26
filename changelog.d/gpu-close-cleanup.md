### Fixed

- Closing a GPU resource now takes its cleanup off the scope it was created under and drops it from the device's retained lists, so recreating resources under a long-lived scope, such as render targets on resize, no longer grows them (#2339 by @owocc).
