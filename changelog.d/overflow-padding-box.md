### Changed

- Clip `Overflow: Hidden` children inside the view's border, to its padding box as CSS does, on Apple, Android, Windows, Linux, watchOS and the terminal, so a bare `ui.Dialog` clips in its surface with no inner view (#1362 by @oneo-me).
