### Fixed

- Remove closed GPU resources from their scope's cleanup set and the device's retained lists so recreating resources under a long-lived scope does not grow them (#2339 by @owocc).
