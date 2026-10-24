// Package layout is NSGo's layout engine: a Go reimplementation of the CSS
// layout algorithms of Taffy 0.15 with the features NSH enables (block,
// flexbox, grid and content sizes; no floats, no calc(), no balanced flex
// wrapping).
//
// The caller implements [Node] over its own tree, then calls [Compute] on a
// layout root and, to snap to whole pixels as Taffy does by default,
// [Round]. Every node's [State] then holds its layout. Each root is laid out
// independently, so separate roots (a list's cells, an overlay) can be
// computed on their own.
//
// The algorithms follow Taffy's source closely, function by function, so that
// they produce the same results; the fixture tests in this package run
// Taffy's own XML test suite to prove it. Comments in the algorithm files keep
// Taffy's references to the CSS specifications.
//
// All arithmetic is float32, as in Taffy. [Opt] stands in for Rust's
// Option<f32>.
package layout
