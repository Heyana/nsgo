package terminal

// boxLines follows Unicode box-drawing names (U+2500–U+257F). The arms
// are left, right, up, down: 1 light, 2 heavy, 3 double. The last value
// identifies diagonals (1–3), rounded corners (4), and dash counts (5–7).
var boxLines = [...]struct{ left, right, up, down, kind uint8 }{
	{1, 1, 0, 0, 0}, // ─ light horizontal
	{2, 2, 0, 0, 0}, // ━ heavy horizontal
	{0, 0, 1, 1, 0}, // │ light vertical
	{0, 0, 2, 2, 0}, // ┃ heavy vertical
	{1, 1, 0, 0, 6}, // ┄ light triple dash horizontal
	{2, 2, 0, 0, 6}, // ┅ heavy triple dash horizontal
	{0, 0, 1, 1, 6}, // ┆ light triple dash vertical
	{0, 0, 2, 2, 6}, // ┇ heavy triple dash vertical
	{1, 1, 0, 0, 7}, // ┈ light quadruple dash horizontal
	{2, 2, 0, 0, 7}, // ┉ heavy quadruple dash horizontal
	{0, 0, 1, 1, 7}, // ┊ light quadruple dash vertical
	{0, 0, 2, 2, 7}, // ┋ heavy quadruple dash vertical
	{0, 1, 0, 1, 0}, // ┌ light down and right
	{0, 2, 0, 1, 0}, // ┍ down light and right heavy
	{0, 1, 0, 2, 0}, // ┎ down heavy and right light
	{0, 2, 0, 2, 0}, // ┏ heavy down and right
	{1, 0, 0, 1, 0}, // ┐ light down and left
	{2, 0, 0, 1, 0}, // ┑ down light and left heavy
	{1, 0, 0, 2, 0}, // ┒ down heavy and left light
	{2, 0, 0, 2, 0}, // ┓ heavy down and left
	{0, 1, 1, 0, 0}, // └ light up and right
	{0, 2, 1, 0, 0}, // ┕ up light and right heavy
	{0, 1, 2, 0, 0}, // ┖ up heavy and right light
	{0, 2, 2, 0, 0}, // ┗ heavy up and right
	{1, 0, 1, 0, 0}, // ┘ light up and left
	{2, 0, 1, 0, 0}, // ┙ up light and left heavy
	{1, 0, 2, 0, 0}, // ┚ up heavy and left light
	{2, 0, 2, 0, 0}, // ┛ heavy up and left
	{0, 1, 1, 1, 0}, // ├ light vertical and right
	{0, 2, 1, 1, 0}, // ┝ vertical light and right heavy
	{0, 1, 2, 1, 0}, // ┞ up heavy and right down light
	{0, 1, 1, 2, 0}, // ┟ down heavy and right up light
	{0, 1, 2, 2, 0}, // ┠ vertical heavy and right light
	{0, 2, 2, 1, 0}, // ┡ down light and right up heavy
	{0, 2, 1, 2, 0}, // ┢ up light and right down heavy
	{0, 2, 2, 2, 0}, // ┣ heavy vertical and right
	{1, 0, 1, 1, 0}, // ┤ light vertical and left
	{2, 0, 1, 1, 0}, // ┥ vertical light and left heavy
	{1, 0, 2, 1, 0}, // ┦ up heavy and left down light
	{1, 0, 1, 2, 0}, // ┧ down heavy and left up light
	{1, 0, 2, 2, 0}, // ┨ vertical heavy and left light
	{2, 0, 2, 1, 0}, // ┩ down light and left up heavy
	{2, 0, 1, 2, 0}, // ┪ up light and left down heavy
	{2, 0, 2, 2, 0}, // ┫ heavy vertical and left
	{1, 1, 0, 1, 0}, // ┬ light down and horizontal
	{2, 1, 0, 1, 0}, // ┭ left heavy and right down light
	{1, 2, 0, 1, 0}, // ┮ right heavy and left down light
	{2, 2, 0, 1, 0}, // ┯ down light and horizontal heavy
	{1, 1, 0, 2, 0}, // ┰ down heavy and horizontal light
	{2, 1, 0, 2, 0}, // ┱ right light and left down heavy
	{1, 2, 0, 2, 0}, // ┲ left light and right down heavy
	{2, 2, 0, 2, 0}, // ┳ heavy down and horizontal
	{1, 1, 1, 0, 0}, // ┴ light up and horizontal
	{2, 1, 1, 0, 0}, // ┵ left heavy and right up light
	{1, 2, 1, 0, 0}, // ┶ right heavy and left up light
	{2, 2, 1, 0, 0}, // ┷ up light and horizontal heavy
	{1, 1, 2, 0, 0}, // ┸ up heavy and horizontal light
	{2, 1, 2, 0, 0}, // ┹ right light and left up heavy
	{1, 2, 2, 0, 0}, // ┺ left light and right up heavy
	{2, 2, 2, 0, 0}, // ┻ heavy up and horizontal
	{1, 1, 1, 1, 0}, // ┼ light vertical and horizontal
	{2, 1, 1, 1, 0}, // ┽ left heavy and right vertical light
	{1, 2, 1, 1, 0}, // ┾ right heavy and left vertical light
	{2, 2, 1, 1, 0}, // ┿ vertical light and horizontal heavy
	{1, 1, 2, 1, 0}, // ╀ up heavy and down horizontal light
	{1, 1, 1, 2, 0}, // ╁ down heavy and up horizontal light
	{1, 1, 2, 2, 0}, // ╂ vertical heavy and horizontal light
	{2, 1, 2, 1, 0}, // ╃ left up heavy and right down light
	{1, 2, 2, 1, 0}, // ╄ right up heavy and left down light
	{2, 1, 1, 2, 0}, // ╅ left down heavy and right up light
	{1, 2, 1, 2, 0}, // ╆ right down heavy and left up light
	{2, 2, 2, 1, 0}, // ╇ down light and up horizontal heavy
	{2, 2, 1, 2, 0}, // ╈ up light and down horizontal heavy
	{2, 1, 2, 2, 0}, // ╉ right light and left vertical heavy
	{1, 2, 2, 2, 0}, // ╊ left light and right vertical heavy
	{2, 2, 2, 2, 0}, // ╋ heavy vertical and horizontal
	{1, 1, 0, 0, 5}, // ╌ light double dash horizontal
	{2, 2, 0, 0, 5}, // ╍ heavy double dash horizontal
	{0, 0, 1, 1, 5}, // ╎ light double dash vertical
	{0, 0, 2, 2, 5}, // ╏ heavy double dash vertical
	{3, 3, 0, 0, 0}, // ═ double horizontal
	{0, 0, 3, 3, 0}, // ║ double vertical
	{0, 3, 0, 1, 0}, // ╒ down single and right double
	{0, 1, 0, 3, 0}, // ╓ down double and right single
	{0, 3, 0, 3, 0}, // ╔ double down and right
	{3, 0, 0, 1, 0}, // ╕ down single and left double
	{1, 0, 0, 3, 0}, // ╖ down double and left single
	{3, 0, 0, 3, 0}, // ╗ double down and left
	{0, 3, 1, 0, 0}, // ╘ up single and right double
	{0, 1, 3, 0, 0}, // ╙ up double and right single
	{0, 3, 3, 0, 0}, // ╚ double up and right
	{3, 0, 1, 0, 0}, // ╛ up single and left double
	{1, 0, 3, 0, 0}, // ╜ up double and left single
	{3, 0, 3, 0, 0}, // ╝ double up and left
	{0, 3, 1, 1, 0}, // ╞ vertical single and right double
	{0, 1, 3, 3, 0}, // ╟ vertical double and right single
	{0, 3, 3, 3, 0}, // ╠ double vertical and right
	{3, 0, 1, 1, 0}, // ╡ vertical single and left double
	{1, 0, 3, 3, 0}, // ╢ vertical double and left single
	{3, 0, 3, 3, 0}, // ╣ double vertical and left
	{3, 3, 0, 1, 0}, // ╤ down single and horizontal double
	{1, 1, 0, 3, 0}, // ╥ down double and horizontal single
	{3, 3, 0, 3, 0}, // ╦ double down and horizontal
	{3, 3, 1, 0, 0}, // ╧ up single and horizontal double
	{1, 1, 3, 0, 0}, // ╨ up double and horizontal single
	{3, 3, 3, 0, 0}, // ╩ double up and horizontal
	{3, 3, 1, 1, 0}, // ╪ vertical single and horizontal double
	{1, 1, 3, 3, 0}, // ╫ vertical double and horizontal single
	{3, 3, 3, 3, 0}, // ╬ double vertical and horizontal
	{0, 1, 0, 1, 4}, // ╭ light arc down and right
	{1, 0, 0, 1, 4}, // ╮ light arc down and left
	{1, 0, 1, 0, 4}, // ╯ light arc up and left
	{0, 1, 1, 0, 4}, // ╰ light arc up and right
	{0, 0, 0, 0, 1}, // ╱ light diagonal upper right to lower left
	{0, 0, 0, 0, 2}, // ╲ light diagonal upper left to lower right
	{0, 0, 0, 0, 3}, // ╳ light diagonal cross
	{1, 0, 0, 0, 0}, // ╴ light left
	{0, 0, 1, 0, 0}, // ╵ light up
	{0, 1, 0, 0, 0}, // ╶ light right
	{0, 0, 0, 1, 0}, // ╷ light down
	{2, 0, 0, 0, 0}, // ╸ heavy left
	{0, 0, 2, 0, 0}, // ╹ heavy up
	{0, 2, 0, 0, 0}, // ╺ heavy right
	{0, 0, 0, 2, 0}, // ╻ heavy down
	{1, 2, 0, 0, 0}, // ╼ light left and heavy right
	{0, 0, 1, 2, 0}, // ╽ light up and heavy down
	{2, 1, 0, 0, 0}, // ╾ heavy left and light right
	{0, 0, 2, 1, 0}, // ╿ heavy up and light down
}
