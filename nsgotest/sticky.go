package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// StickyCell records a sticky cell and its travel in a scroll view.
type StickyCell struct {
	Cell       core.NodeID
	Start, End core.Points
	// Node optionally animates the cell's inset from the viewport start.
	// Start includes the static Inset.
	Node  uint32
	Inset core.Points
}

func stickyCell(c driver.StickyCell) StickyCell {
	return StickyCell{
		Cell:  c.Cell,
		Start: c.Start,
		End:   c.End,
		Node:  c.Node,
		Inset: c.Inset,
	}
}

func (c StickyCell) stickyCell() driver.StickyCell {
	return driver.StickyCell{
		Cell:  c.Cell,
		Start: c.Start,
		End:   c.End,
		Node:  c.Node,
		Inset: c.Inset,
	}
}

// StickyShift returns the cell's static shift at the given scroll offset.
func (c StickyCell) StickyShift(offset core.Points) core.Points {
	return c.stickyCell().StickyShift(offset)
}

func stickyCells(cells []driver.StickyCell) []StickyCell {
	if cells == nil {
		return nil
	}
	result := make([]StickyCell, len(cells))
	for i, cell := range cells {
		result[i] = stickyCell(cell)
	}
	return result
}
