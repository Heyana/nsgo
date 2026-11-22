### Added

- Mount DevTools' overlay (update highlights, Pick and the layout audit) in every surface of an `nsgo dev` build — including `desktop.OpenWindow` secondary windows, `desktop.OpenPanel` panels, `nsgo.NewView` embedded views and iOS `ModalPageSheet`/`ModalFormSheet` sheets — synchronized through a shared controller and cleanly unregistered on surface close (#1616).
