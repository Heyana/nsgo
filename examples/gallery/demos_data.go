//go:build !gallery_group || gallery_data

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 4, "Data", m.GroupData, []demo{
		{"List", m.DemoList, m.DemoListDetail, lucide.List, cardsDemo(listCard, treeCard)},
		{"Table", m.DemoTable, m.DemoTableDetail, lucide.Table, cardsDemo(tableCard)},
		{"DataTable", m.DemoDataTable, m.DemoDataTableDetail, lucide.Table, dataTableDemo},
		{"CommitGraph", m.DemoCommitGraph, m.DemoCommitGraphDetail, lucide.GitGraph, commitGraphDemo},
		{"Plot", m.DemoPlot, m.DemoPlotDetail, lucide.ChartSpline, chartsDemo},
		{"FlowDiagram", m.DemoFlowDiagram, m.DemoFlowDiagramDetail, lucide.Workflow, flowDemo},
	})
}
