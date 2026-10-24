# Taffy fixtures

`taffy-xml.tar.gz` holds the XML layout fixtures of [Taffy](https://github.com/DioxusLabs/taffy) at tag `v0.15.0` (`tests/xml/`), without the `float` category, which NSH disables. Each fixture is an input tree with expectations derived from Chrome; `fixtures_test.go` runs them all. They are Taffy's, under the MIT license in `LICENSE`.

To rebuild the archive, from a clone of Taffy checked out at `v0.15.0`:

```sh
cd tests && COPYFILE_DISABLE=1 tar --no-mac-metadata --exclude 'xml/float' --exclude 'xml/mod.rs' \
  -czf <nsgo>/core/internal/layout/testdata/taffy-xml.tar.gz xml
```
