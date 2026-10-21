package core

// NodeID identifies a node of one tree. IDs start at 1 and are dense.
type NodeID uint32

// ItemKind is what a mount item does.
type ItemKind uint8

const (
	// ItemCreate makes the native view of a new node (ID, Tag).
	ItemCreate ItemKind = iota + 1
	// ItemProps replaces the node's visual props (Props).
	ItemProps
	// ItemInsert puts the node into Parent's children at Index.
	ItemInsert
	// ItemFrame moves and sizes the node, relative to its parent (Frame).
	ItemFrame
)

// Props is everything a platform draws for one node. Commit sends it
// whole when any part changes.
type Props struct {
	Background   Color
	CornerRadius Points
	Pressable    bool
	Role         Role

	// Text and Input only.
	Text        string
	Font        Font
	Color       Color
	Padding     Edges
	Placeholder string
}

// MountItem is one change to the native views.
type MountItem struct {
	Kind   ItemKind
	ID     NodeID
	Tag    Tag
	Parent NodeID
	Index  int
	Frame  Bounds
	Props  Props
}

// MountBatch is one commit's changes, applied in order on the main thread.
// Root is the node the platform shows in its surface.
type MountBatch struct {
	Root  NodeID
	Items []MountItem
}

// TextEngine measures text for layout. The engine calls it on its own
// goroutine, so implementations must not touch main-thread-only objects.
type TextEngine interface {
	// Measure returns the size of text wrapped at maxWidth (Auto: no wrap).
	Measure(text string, font Font, maxWidth Points) Size
}
