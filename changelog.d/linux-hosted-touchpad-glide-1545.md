### Added

- Track touchpad scroll release velocity through the Linux hosted-view shield (and enable kinetic scrolling on `NoScroll` WebKitGTK views) so shielded touchpad scrolls glide and report `OnWheel` end velocities like the surface's own, and stop on touchpad hold or new input (#1545).
