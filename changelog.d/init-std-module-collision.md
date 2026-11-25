### Fixed

- Give a module template's app a module path of its own when its name matches a standard library package, such as `time`, and refuse a `-module` value that names one, instead of generating an app that Go cannot build (#2270 by @Laotree).
