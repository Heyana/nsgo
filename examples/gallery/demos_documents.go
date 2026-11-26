//go:build !gallery_group || gallery_documents

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 9, "Documents", m.GroupDocuments, []demo{
		{"Conversation", m.DemoConversation, m.DemoConversationDetail, lucide.MessagesSquare, conversationDemo},
		{"Editor", m.DemoEditor, m.DemoEditorDetail, lucide.Code, editorDemo},
		{"RichEditor", m.DemoRichEditor, m.DemoRichEditorDetail, lucide.PencilLine, richEditorDemo},
		{"DiffView", m.DemoDiffView, m.DemoDiffViewDetail, lucide.FileDiff, diffDemo},
		{"Terminal", m.DemoTerminal, m.DemoTerminalDetail, lucide.Terminal, terminalDemo},
		{"TextView", m.DemoTextView, m.DemoTextViewDetail, lucide.FileText, textViewDemo},
	})
}
