### Changed

- `nsgo test` takes Go package patterns, so a directory names its own package and `/...` its tree, and each CI test shard runs its packages in one run instead of testing the whole module first (#2504 by @im-apratap).
