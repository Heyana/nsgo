### Changed

- Share interned static styles, layout styles and view snapshots across `core` nodes and allocate rare per-node state lazily, reducing `node` from 6.5 KB to 760 B (#1489).
