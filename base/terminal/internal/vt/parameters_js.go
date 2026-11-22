//go:build js && !nsgominiprogram && !nsgoharmony

package vt

import "fmt"

type wasmParameter struct {
	index int
	typ   string
	count uintptr
}

func wasmParameters(name string, args []uintptr) []wasmParameter {
	one := func(index int, typ string) []wasmParameter { return []wasmParameter{{index, typ, 1}} }
	two := func(a int, at string, b int, bt string) []wasmParameter {
		return []wasmParameter{{a, at, 1}, {b, bt, 1}}
	}
	switch name {
	case "ghostty_type_json", "ghostty_alloc", "ghostty_free", "ghostty_sys_set",
		"ghostty_terminal_free", "ghostty_terminal_reset", "ghostty_terminal_resize",
		"ghostty_render_state_free", "ghostty_render_state_begin_update", "ghostty_render_state_end_update", "ghostty_render_state_clean",
		"ghostty_render_state_row_iterator_free", "ghostty_render_state_row_iterator_next", "ghostty_render_state_row_cells_free", "ghostty_render_state_row_cells_select",
		"ghostty_key_encoder_free", "ghostty_key_encoder_setopt_from_terminal", "ghostty_key_event_free",
		"ghostty_key_event_set_action", "ghostty_key_event_set_key", "ghostty_key_event_set_mods", "ghostty_key_event_set_consumed_mods", "ghostty_key_event_set_composing", "ghostty_key_event_set_unshifted_codepoint",
		"ghostty_mouse_encoder_free", "ghostty_mouse_encoder_setopt_from_terminal", "ghostty_mouse_encoder_reset", "ghostty_mouse_event_free", "ghostty_mouse_event_set_action", "ghostty_mouse_event_set_button", "ghostty_mouse_event_clear_button", "ghostty_mouse_event_set_mods",
		"ghostty_selection_gesture_free", "ghostty_selection_gesture_reset", "ghostty_selection_gesture_event_free",
		"ghostty_kitty_graphics_image", "ghostty_kitty_graphics_placement_iterator_free", "ghostty_kitty_graphics_placement_next",
		"ghostty_formatter_free", "ghostty_search_free", "ghostty_search_feed":
		return nil
	case "ghostty_terminal_new", "ghostty_render_state_new", "ghostty_render_state_row_iterator_new", "ghostty_render_state_row_cells_new",
		"ghostty_key_encoder_new", "ghostty_key_event_new", "ghostty_mouse_encoder_new", "ghostty_mouse_event_new",
		"ghostty_selection_gesture_new", "ghostty_selection_gesture_event_new", "ghostty_kitty_graphics_placement_iterator_new", "ghostty_search_new":
		return one(1, "pointer")
	case "ghostty_terminal_vt_write", "ghostty_key_event_set_utf8":
		return []wasmParameter{{1, "u8", args[2]}}
	case "ghostty_paste_is_safe":
		return []wasmParameter{{0, "u8", args[1]}}
	case "ghostty_terminal_set":
		typ := map[uintptr]string{
			optColorForeground: "GhosttyColorRgb", optColorBackground: "GhosttyColorRgb", optColorCursor: "GhosttyColorRgb",
			optKittyImageLimit: "u64", optDefaultCursorStyle: "i32", optDefaultCursorBlink: "bool", optScrollbackMaxBytes: "usize",
			optMode: "GhosttyTerminalModeConfig", optModeDefault: "GhosttyTerminalModeConfig", optTerminfoName: "GhosttyString", optResizePullBack: "bool", 21: "GhosttySelection",
		}[args[1]]
		if args[1] == optColorPalette {
			return []wasmParameter{{2, "GhosttyColorRgb", 256}}
		}
		if typ != "" {
			return one(2, typ)
		}
		if args[1] == optUserdata || args[1] == optWritePTY || args[1] == optBell || args[1] == optTitleChanged || args[1] == optPwdChanged || args[1] == optRenderHold || args[1] == optClipboardWrite || args[1] == optClipboardRead || args[1] == optSize || args[1] == optColorScheme || args[1] == optDeviceAttributes || args[1] == optDesktopNotification || args[1] == optProgressReport {
			return nil
		}
	case "ghostty_terminal_get":
		// CURSOR_AT_PROMPT is another bool output, independent of host size_t.
		if args[1] == 39 {
			return one(2, "bool")
		}
		typ := map[uintptr]string{
			dataCols: "u16", dataRows: "u16", dataCursorX: "u16", dataCursorY: "u16", dataActiveScreen: "i32", dataKittyKeyboardFlags: "u8", dataScrollbar: "GhosttyTerminalScrollbar", dataMouseTracking: "bool", dataTitle: "GhosttyString", dataPwd: "GhosttyString", dataScrollbackRows: "usize", dataViewportActive: "bool", dataMode: "GhosttyTerminalModeConfig", dataMouseShape: "i32", dataKittyGraphics: "pointer", 31: "GhosttySelection",
		}[args[1]]
		if args[1] >= dataColorFg && args[1] < dataColorFg+3 || args[1] >= dataColorFgDefault && args[1] < dataColorFgDefault+3 {
			typ = "GhosttyColorRgb"
		}
		if args[1] == dataColorFg+3 || args[1] == dataColorFgDefault+3 {
			return []wasmParameter{{2, "GhosttyColorRgb", 256}}
		}
		if typ != "" {
			return one(2, typ)
		}
	case "ghostty_terminal_compress":
		return one(2, "i32")
	case "ghostty_terminal_compression_activity":
		return one(1, "u64")
	case "ghostty_terminal_scroll_viewport":
		return one(1, "GhosttyTerminalScrollViewport")
	case "ghostty_terminal_grid_ref":
		return two(1, "GhosttyPoint", 2, "GhosttyGridRef")
	case "ghostty_render_state_get":
		typ := map[uintptr]string{rsDataCols: "u16", rsDataRows: "u16", rsDataDirty: "i32", rsDataRowIterator: "pointer", rsDataCursor: "GhosttyRenderStateCursor", rsDataColors: "GhosttyRenderStateColors"}[args[1]]
		if typ != "" {
			return one(2, typ)
		}
	case "ghostty_render_state_row_get":
		typ := map[uintptr]string{rowDataDirty: "bool", rowDataRaw: "u64", rowDataCells: "pointer", rowDataSelection: "GhosttyRenderStateRowSelection", rowDataCellsRaw: "GhosttyCellsView", rowDataID: "GhosttyRenderStateRowId"}[args[1]]
		if typ != "" {
			return one(2, typ)
		}
	case "ghostty_render_state_row_cells_get":
		if args[1] == cellDataStyle {
			return one(2, "GhosttyStyle")
		}
		if args[1] == cellDataGraphemesLen {
			return one(2, "u32")
		}
		if args[1] == cellDataGraphemesBuf {
			n := wasm.alloc.Invoke(4).Int()
			defer wasm.free.Invoke(n, 4)
			wasm.exports.Get(name).Invoke(int(args[0]), cellDataGraphemesLen, n)
			return []wasmParameter{{2, "u32", uintptr(unsignedABI(foreignBytes(uintptr(n), 4)))}}
		}
	case "ghostty_row_get":
		if args[1] == rowDataWrap {
			return one(2, "bool")
		}
		if args[1] == rowDataSemanticPrompt {
			return one(2, "i32")
		}
	case "ghostty_key_encoder_setopt":
		if args[1] == 5 {
			return one(2, "u8")
		}
		if args[1] == 6 {
			return one(2, "i32")
		}
	case "ghostty_mouse_encoder_setopt":
		typ := map[uintptr]string{2: "GhosttyMouseEncoderSize", 3: "bool", 4: "bool"}[args[1]]
		if typ != "" {
			return one(2, typ)
		}
	case "ghostty_mouse_event_set_position":
		return one(1, "GhosttyMousePosition")
	case "ghostty_key_encoder_encode", "ghostty_mouse_encoder_encode":
		return []wasmParameter{{2, "u8", args[3]}, {4, "usize", 1}}
	case "ghostty_focus_encode":
		return []wasmParameter{{1, "u8", args[2]}, {3, "usize", 1}}
	case "ghostty_paste_encode":
		return []wasmParameter{{0, "u8", args[1]}, {3, "u8", args[4]}, {5, "usize", 1}}
	case "ghostty_terminal_paste":
		return two(1, "GhosttyPaste", 2, "bool")
	case "ghostty_selection_gesture_get":
		if args[2] == gestureDataDragged {
			return one(3, "bool")
		}
		if args[2] == gestureDataAutoscroll {
			return one(3, "i32")
		}
	case "ghostty_selection_gesture_event":
		return one(3, "GhosttySelection")
	case "ghostty_selection_gesture_event_set":
		typ := map[uintptr]string{gestureOptRef: "GhosttyGridRef", gestureOptPosition: "GhosttySurfacePosition", gestureOptRepeatDistance: "f64", gestureOptTime: "u64", gestureOptRepeatInterval: "u64", gestureOptRectangle: "bool", gestureOptGeometry: "GhosttySelectionGestureGeometry", gestureOptViewport: "GhosttyPointCoordinate"}[args[1]]
		if typ != "" {
			return one(2, typ)
		}
	case "ghostty_terminal_select_all":
		return one(1, "GhosttySelection")
	case "ghostty_terminal_selection_format_alloc":
		return []wasmParameter{{2, "GhosttyTerminalSelectionFormatOptions", 1}, {3, "pointer", 1}, {4, "usize", 1}}
	case "ghostty_grid_ref_hyperlink_uri":
		return []wasmParameter{{0, "GhosttyGridRef", 1}, {1, "u8", args[2]}, {3, "usize", 1}}
	case "ghostty_grid_ref_graphemes":
		return []wasmParameter{{0, "GhosttyGridRef", 1}, {1, "u32", args[2]}, {3, "usize", 1}}
	case "ghostty_grid_ref_cell", "ghostty_grid_ref_row":
		return two(0, "GhosttyGridRef", 1, "u64")
	case "ghostty_terminal_point_from_grid_ref":
		return two(1, "GhosttyGridRef", 3, "GhosttyPointCoordinate")
	case "ghostty_formatter_terminal_new":
		return two(1, "pointer", 3, "GhosttyFormatterTerminalOptions")
	case "ghostty_formatter_format_alloc":
		return two(2, "pointer", 3, "usize")
	case "ghostty_kitty_graphics_get":
		if args[1] == kittyPlacementIterator {
			return one(2, "pointer")
		}
	case "ghostty_kitty_graphics_image_get":
		typ := map[uintptr]string{imageWidth: "u32", imageHeight: "u32", imageFormat: "i32", imageData: "pointer", imageDataLen: "usize", imageGeneration: "u64"}[args[1]]
		if typ != "" {
			return one(2, typ)
		}
	case "ghostty_kitty_graphics_placement_get":
		if args[1] == placementVirtual {
			return one(2, "bool")
		}
		return one(2, "u32")
	case "ghostty_kitty_graphics_placement_render_info":
		return one(3, "GhosttyKittyGraphicsPlacementRenderInfo")
	case "ghostty_search_set":
		if args[1] == searchOptNeedle {
			return one(2, "GhosttyString")
		}
		if args[1] == searchOptSelectScroll {
			return one(2, "i32")
		}
		if args[1] == searchOptSelectNext || args[1] == searchOptSelectPrev {
			return nil
		}
	case "ghostty_search_tick":
		return one(1, "i32")
	case "ghostty_search_get":
		typ := map[uintptr]string{searchDataStatus: "i32", searchDataTotal: "usize", searchDataSelectedIndex: "usize", searchDataSelectedMatch: "GhosttySelection", searchDataViewportMatches: "GhosttySelectionBuffer"}[args[1]]
		if typ != "" {
			return one(2, typ)
		}
	}
	panic(fmt.Sprintf("vt: no WebAssembly pointer schema for %s %v", name, args))
}
