package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/desktop"
)

var featured = []struct {
	id   string
	icon iconName
	note string
}{{"guide", iconText, "Markdown, a table, code"}, {"reason", iconSpark, "Reasoning first"}, {"search", iconSearch, "A tool call"}, {"vision", iconImage, "A photo and a file"}}

// chatRun is the reply being written.
type chatRun struct {
	id           string
	model        chatModel
	script       string
	cancelled    bool
	md, reason   string
	thinkStart   time.Time
	thinkSeconds int
}

// chatDemo is a chat assistant backed by the local Claude or Codex CLI on
// desktops, and by scripted streaming scenarios on the web and phones. The
// desktop layout adds a list of conversations on the left and keeps the
// messages and the composer in a readable column.
func chatDemo(cx *core.Scope) core.Node {
	env := cx.Env()
	wide := shell.desktop()
	column := core.Style{}
	if wide {
		column = core.Style{Width: core.Pct(100), MaxWidth: core.Pt(820), AlignSelf: core.AlignCenter}
	}
	ids := 0
	nextID := func() string { ids++; return fmt.Sprintf("m%d", ids) }

	messages := core.NewSignalFunc(cx, []msg(nil), func(a, b []msg) bool { return false })
	model := core.NewSignal(cx, chatModel("Swift"))
	native := desktop.IsAvailable(cx)
	provider := core.NewSignal(cx, "Codex")
	var cancelLocal func()
	draft := core.NewSignal(cx, "")
	inputHeight := core.NewSignal(cx, core.Points(20))
	attachment := core.NewSignalFunc(cx, (*chatAttachment)(nil), func(a, b *chatAttachment) bool { return a == b })
	menuOpen := core.NewSignal(cx, false)
	bannerOn := core.NewSignal(cx, false)
	bannerKind := core.NewSignal(cx, "offline")
	// copied is the copied message, for a moment.
	copied := core.NewSignal(cx, "")

	// The reply being written; finished replies are plain data in
	// messages.
	liveID := core.NewSignal(cx, "")
	liveReason, liveMd := core.NewSignal(cx, ""), core.NewSignal(cx, "")
	thinking := core.NewSignal(cx, false)
	thinkSeconds := core.NewSignal(cx, 0)
	liveTools := core.NewSignalFunc(cx, []chatTool(nil), func(a, b []chatTool) bool { return false })
	now := core.NewSignal(cx, time.Time{})

	// following is whether the user is at the bottom: the list then
	// follows the stream; scrolling up clears it.
	following := core.NewSignal(cx, true)
	list := &core.Ref{}
	dragging := false
	var run *chatRun
	var ticker func()

	toEnd := func(animated bool) {
		following.Set(true)
		list.ScrollToEnd(animated)
	}
	after := func(r *chatRun, d time.Duration, fn func()) {
		later(cx, d, func() {
			if !r.cancelled {
				fn()
			}
		})
	}
	// stream writes text at about rate tokens a second, with jitter.
	stream := func(r *chatRun, text string, rate float64, intoReason bool, then func()) {
		tokens := streamTokens(text)
		var next func(i int)
		next = func(i int) {
			if i >= len(tokens) {
				then()
				return
			}
			after(r, time.Duration(1000/rate*(0.55+rand.Float64()*0.9))*time.Millisecond, func() {
				if intoReason {
					r.reason += tokens[i]
					liveReason.Set(r.reason)
				} else {
					r.md += tokens[i]
					liveMd.Set(r.md)
				}
				next(i + 1)
			})
		}
		next(0)
	}
	setTool := func(id string, st toolState) {
		liveTools.Update(func(ts []chatTool) []chatTool {
			ts = slices.Clone(ts)
			for i := range ts {
				if ts[i].id == id {
					ts[i].state = st
				}
			}
			return ts
		})
	}
	stopTicker := func() {
		if ticker != nil {
			ticker()
			ticker = nil
		}
	}
	replace := func(id string, m msg) {
		messages.Update(func(ms []msg) []msg {
			ms = slices.Clone(ms)
			for i := range ms {
				if ms[i].id == id {
					ms[i] = m
				}
			}
			return ms
		})
	}
	// freeze turns the reply into a message and ends the run.
	freeze := func(r *chatRun, status, errText string) {
		if run != r {
			return
		}
		stopTicker()
		seconds := r.thinkSeconds
		if seconds == 0 {
			seconds = max(1, int(time.Since(r.thinkStart).Seconds()+0.5))
		}
		final := msg{kind: "assistant", id: r.id, model: r.model, script: r.script, md: r.md, status: status, err: errText}
		if r.reason != "" {
			final.reasoning = &reasoning{text: r.reason, seconds: seconds}
		}
		for _, t := range liveTools.GetUntracked() {
			if t.state != toolDone {
				t.state = toolCancelled
			}
			final.tools = append(final.tools, t)
		}
		run = nil
		cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
		cx.Batch(func() {
			replace(r.id, final)
			liveID.Set("")
		})
		// Once the layout animation has settled, a following list ends
		// exactly at the bottom.
		later(cx, 360*time.Millisecond, func() {
			if following.GetUntracked() {
				list.ScrollToEnd(true)
			}
		})
	}

	// reply runs a scenario's reply; at replaces that message (retry,
	// regenerate).
	var reply func(s scenario, at string)
	reply = func(s scenario, at string) {
		script := s.id
		if native {
			script = "local:" + provider.GetUntracked()
		}
		r := &chatRun{id: nextID(), model: s.model, script: script, thinkStart: time.Now()}
		run = r
		placeholder := msg{kind: "assistant", id: r.id, model: s.model, script: script, status: "done"}
		cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
		cx.Batch(func() {
			liveReason.Set("")
			thinking.Set(false)
			thinkSeconds.Set(0)
			liveTools.Set(nil)
			liveMd.Set("")
			liveID.Set(r.id)
			if at != "" {
				replace(at, placeholder)
			} else {
				messages.Update(func(ms []msg) []msg { return append(slices.Clone(ms), placeholder) })
			}
		})
		if native {
			var parts []string
			for _, m := range messages.GetUntracked() {
				if m.id == r.id {
					break
				}
				switch {
				case m.kind == "user":
					parts = append(parts, "User: "+m.text)
				case m.kind == "assistant" && m.id != at:
					parts = append(parts, "Assistant: "+m.md)
				}
			}
			prompt := "You are a helpful chat assistant. Answer the final user message using the conversation below. Reply directly in Markdown. Do not use tools or modify files.\n\n" + strings.Join(parts, "\n\n")
			cancelLocal = localReply(cx, provider.GetUntracked(), prompt, func(chunk string) {
				if run != r || r.cancelled {
					return
				}
				r.md += chunk
				liveMd.Set(r.md)
			}, func(errText string) {
				if run != r || r.cancelled {
					return
				}
				cancelLocal = nil
				if errText != "" {
					freeze(r, "error", errText)
				} else {
					freeze(r, "done", "")
				}
			})
			return
		}
		var step func(i int)
		step = func(i int) {
			if i >= len(s.steps) {
				freeze(r, "done", "")
				return
			}
			st := s.steps[i]
			switch {
			case st.think != "":
				r.thinkStart = time.Now()
				now.Set(r.thinkStart)
				thinking.Set(true)
				ticker = every(cx, time.Second, func() { now.Set(time.Now()) })
				stream(r, st.think, 75, true, func() {
					stopTicker()
					r.thinkSeconds = max(1, int(time.Since(r.thinkStart).Seconds()+0.5))
					thinkSeconds.Set(r.thinkSeconds)
					thinking.Set(false)
					after(r, 350*time.Millisecond, func() { step(i + 1) })
				})
			case st.tool != nil:
				t := *st.tool
				t.state = toolPending
				liveTools.Update(func(ts []chatTool) []chatTool { return append(slices.Clone(ts), t) })
				after(r, 450*time.Millisecond, func() {
					setTool(t.id, toolRunning)
					after(r, time.Duration(st.ms)*time.Millisecond, func() {
						cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
						setTool(t.id, toolDone)
						after(r, 400*time.Millisecond, func() { step(i + 1) })
					})
				})
			case st.say != "":
				stream(r, st.say, 40, false, func() { step(i + 1) })
			case st.fail != "":
				after(r, 900*time.Millisecond, func() { freeze(r, "error", st.fail) })
			default:
				step(i + 1)
			}
		}
		after(r, 450*time.Millisecond, func() { step(0) })
	}

	stop := func() {
		if run == nil {
			return
		}
		run.cancelled = true
		if cancelLocal != nil {
			cancelLocal()
			cancelLocal = nil
		}
		freeze(run, "stopped", "")
	}
	cx.Cleanup(func() {
		if run != nil {
			run.cancelled = true
		}
		if cancelLocal != nil {
			cancelLocal()
		}
		stopTicker()
	})

	// submit sends the user's message for a scenario, then its reply.
	submit := func(s scenario, text string, image core.ImageSource, file *fileRef) {
		if run != nil {
			return
		}
		menuOpen.Set(false)
		following.Set(true)
		offline := 0
		if !native {
			for _, st := range s.steps {
				if st.offline > 0 {
					offline = st.offline
				}
			}
		}
		user := msg{kind: "user", id: nextID(), text: text, image: image, file: file, waiting: offline > 0}
		cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
		cx.Batch(func() {
			messages.Update(func(ms []msg) []msg {
				ms = slices.Clone(ms)
				if len(ms) == 0 {
					ms = append(ms, msg{kind: "date", id: nextID(), label: "Today"})
				}
				return append(ms, user)
			})
			draft.Set("")
			attachment.Set(nil)
			model.Set(s.model)
		})
		toEnd(true)
		var steps []chatStep
		for _, st := range s.steps {
			if st.offline == 0 {
				steps = append(steps, st)
			}
		}
		s.steps = steps
		if offline == 0 {
			reply(s, "")
			return
		}
		bannerKind.Set("offline")
		bannerOn.Set(true)
		later(cx, time.Duration(offline)*time.Millisecond, func() {
			bannerKind.Set("online")
			u := user
			u.waiting = false
			replace(user.id, u)
			later(cx, 900*time.Millisecond, func() {
				bannerOn.Set(false)
				reply(s, "")
			})
		})
	}
	send := func() {
		text := strings.TrimSpace(draft.GetUntracked())
		a := attachment.GetUntracked()
		if text == "" && a == nil {
			return
		}
		if a != nil {
			v, _ := findScenario("vision")
			if m := model.GetUntracked(); m != "Swift" {
				v.model = m
			}
			if text == "" {
				text = v.prompt
			}
			submit(v, text, a.image, &a.file)
			return
		}
		submit(freeform(text, model.GetUntracked()), text, core.ImageSource{}, nil)
	}
	play := func(id string) {
		if id == "history" {
			menuOpen.Set(false)
			messages.Update(func(ms []msg) []msg { return append(chatHistory(nextID), ms...) })
			toEnd(false)
			return
		}
		s, _ := findScenario(id)
		submit(s, s.prompt, s.image, s.file)
	}
	update := func(id string, change func(m msg) msg) {
		messages.Update(func(ms []msg) []msg {
			ms = slices.Clone(ms)
			for i := range ms {
				if ms[i].id == id && ms[i].kind == "assistant" {
					ms[i] = change(ms[i])
				}
			}
			return ms
		})
	}
	promptBefore := func(id string) string {
		ms := messages.GetUntracked()
		i := slices.IndexFunc(ms, func(m msg) bool { return m.id == id })
		for i--; i >= 0; i-- {
			if ms[i].kind == "user" {
				return ms[i].text
			}
		}
		return ""
	}
	regenerate := func(m msg) {
		if run != nil {
			return
		}
		following.Set(true)
		s, ok := findScenario(m.script)
		switch {
		case m.script == "retry":
			s = retried
		case !ok:
			s = freeform(promptBefore(m.id), m.model)
		}
		s.model = m.model
		var steps []chatStep
		for _, st := range s.steps {
			if st.offline == 0 && st.fail == "" {
				steps = append(steps, st)
			}
		}
		s.steps = steps
		reply(s, m.id)
	}
	retry := func(m msg) {
		if run != nil {
			return
		}
		following.Set(true)
		if native {
			regenerate(m)
		} else {
			reply(retried, m.id)
		}
	}
	copyText := func(key, text string) {
		cx.SetClipboard(text)
		copied.Set(key)
		later(cx, 1600*time.Millisecond, func() {
			if copied.GetUntracked() == key {
				copied.Set("")
			}
		})
	}

	// Motion, run on the platform when the state changes.
	springTo := func(on func() bool) *core.AnimatedValue {
		v := core.NewAnimatedValue(cx, 0)
		cx.Effect(func() {
			to := 0.0
			if on() {
				to = 1
			}
			spring(cx, v, to, settleSpring)
		})
		return v
	}
	jump := springTo(func() bool { return !following.Get() })
	menuValue := springTo(menuOpen.Get)
	bannerValue := springTo(bannerOn.Get)

	streaming := func() bool { return liveID.Get() != "" }
	canSend := func() bool { return strings.TrimSpace(draft.Get()) != "" || attachment.Get() != nil }

	userRow := func(m func() msg) core.Node {
		return core.View{Style: core.Style{AlignItems: core.AlignEnd, Padding: core.Symmetric(8, 16), Gap: 8}, Children: []core.Node{
			core.Show{When: func() bool { return m().image.URI != "" || m().image.Data != nil }, Then: func(*core.Scope) core.Node { return attachedPhoto(m().image, 220, 165) }},
			core.Show{When: func() bool { return m().file != nil }, Then: func(*core.Scope) core.Node { return fileChip(*m().file, nil) }},
			core.Show{When: func() bool { return m().text != "" }, Then: func(*core.Scope) core.Node {
				// A message is read, not pressed: flat, a hairline around it.
				return core.View{Style: core.Style{MaxWidth: core.Pct(82), Padding: core.Symmetric(10, 14), CornerRadii: core.Corners{TopLeft: 20, TopRight: 20, BottomRight: 6, BottomLeft: 20},
					BorderCurve: core.BorderCurveContinuous, Background: colors.raised, BorderWidth: 1, BorderColor: colors.border}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return m().text }, Selectable: core.SelectableOn, Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.text}},
				}}
			}},
			core.Show{When: func() bool { return m().waiting }, Then: func(*core.Scope) core.Node {
				return hrow(6, spinner(12, colors.warn), text("Waiting for a connection", 12, regular, colors.muted))
			}},
		}}
	}

	assistantRow := func(m func() msg) core.Node {
		live := func() bool { return liveID.Get() == m().id }
		reason := func() (string, bool, bool) {
			if live() {
				t := liveReason.Get()
				return t, t != "", true
			}
			if r := m().reasoning; r != nil {
				return r.text, true, r.open
			}
			return "", false, false
		}
		thinkingLabel := func() string {
			if live() {
				if thinking.Get() {
					return fmt.Sprintf("Thinking… %d s", max(0, int(now.Get().Sub(run.thinkStart).Seconds())))
				}
				return fmt.Sprintf("Thought for %d s", max(1, thinkSeconds.Get()))
			}
			if r := m().reasoning; r != nil {
				return fmt.Sprintf("Thought for %d s", r.seconds)
			}
			return ""
		}
		tools := func() []chatTool {
			if live() {
				return liveTools.Get()
			}
			return m().tools
		}
		md := func() string {
			if live() {
				return liveMd.Get()
			}
			return m().md
		}
		waitingForWords := func() bool { return live() && liveMd.Get() == "" && len(liveTools.Get()) == 0 && !thinking.Get() }
		done := func() bool { return !live() }
		return core.View{Style: core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: 4, Left: 16}, Gap: 12}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Height: core.Pt(26)}, Children: []core.Node{
				avatar(),
				core.Text{ContentFunc: func() string {
					if s, ok := strings.CutPrefix(m().script, "local:"); ok {
						return s
					}
					return string(m().model)
				}, Style: core.Style{FlexGrow: 1, FontSize: 13, FontWeight: semibold, Color: colors.muted}},
				core.Show{When: func() bool { return done() && m().status == "stopped" }, Then: func(*core.Scope) core.Node { return badge("Stopped", toneWarn) }},
			}},
			core.Show{When: func() bool { _, ok, _ := reason(); return ok }, Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					press(pressProps{scale: 0.98, label: "Reasoning", selected: func() bool { _, _, o := reason(); return o }, onPress: func() {
						if core.Untracked(cx, live) {
							return
						}
						cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)
						update(core.Untracked(cx, m).id, func(x msg) msg {
							if x.reasoning != nil {
								r := *x.reasoning
								r.open = !r.open
								x.reasoning = &r
							}
							return x
						})
					}}, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Height: core.Pt(32), AlignSelf: core.AlignStart}, Children: []core.Node{
						core.Dynamic(func(*core.Scope) core.Node {
							if live() && thinking.Get() {
								return spinner(14, colors.muted)
							}
							return icon(glyphBrain, 16, colors.muted)
						}),
						textf(thinkingLabel, 14, regular, colors.muted),
						core.View{StyleFunc: func() core.Style {
							s := core.Style{Transition: core.Transition{Duration: ms(220), Easing: core.EaseOut, Properties: core.TransitionTransform}}
							if _, _, o := reason(); !o {
								s.Transform.Rotate = -90
							}
							return s
						}, Children: []core.Node{icon(glyphCaretDown, 14, colors.faint)}},
					}}),
					core.Show{When: func() bool { _, _, o := reason(); return o }, Then: func(*core.Scope) core.Node {
						return core.View{Style: core.Style{Direction: core.Row, Gap: 12, Padding: core.Edges{Left: 7}}, Children: []core.Node{
							core.View{Style: core.Style{Width: core.Pt(2), CornerRadius: 1, Background: colors.border}},
							core.Text{ContentFunc: func() string { t, _, _ := reason(); return t }, Selectable: core.SelectableOn,
								Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, LineHeight: 20, Color: colors.muted}},
						}}
					}},
				}}
			}},
			core.Each(tools, func(t chatTool) string { return t.id }, func(cx *core.Scope, t func() chatTool, _ func() int) core.Node { return toolCard(t) }),
			core.Show{When: waitingForWords, Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(22), AlignItems: core.AlignCenter}, Children: []core.Node{caret()}}
			}},
			chatMarkdown(md, live),
			core.Show{When: func() bool { return done() && m().status == "error" }, Then: func(*core.Scope) core.Node {
				return errorRow(func() string {
					if e := m().err; e != "" {
						return e
					}
					return "Something went wrong."
				}, func() { retry(core.Untracked(cx, m)) })
			}},
			core.Show{When: func() bool { return done() && m().status == "stopped" }, Then: func(*core.Scope) core.Node {
				return hrow(8, core.View{Style: core.Style{Width: core.Pt(10), Height: core.Pt(10), CornerRadius: 2.5, Background: colors.faint}}, text("You stopped this reply", 13, regular, colors.faint))
			}},
			core.Show{When: func() bool { return done() && m().status != "error" }, Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Margin: core.Edges{Left: -8, Top: -6}}, Children: []core.Node{
					actionButton(func() iconName {
						if copied.Get() == m().id {
							return iconCheck
						}
						return iconCopy
					}, "Copy", func() bool { return copied.Get() == m().id }, toneUp, func() { mm := core.Untracked(cx, m); copyText(mm.id, mm.md) }),
					actionButton(func() iconName { return glyphRegenerate }, "Regenerate", func() bool { return false }, toneAccent, func() { regenerate(core.Untracked(cx, m)) }),
					actionButton(func() iconName { return glyphThumbUp }, "Good reply", func() bool { return m().rating == "up" }, toneAccent, func() {
						update(core.Untracked(cx, m).id, func(x msg) msg {
							if x.rating == "up" {
								x.rating = ""
							} else {
								x.rating = "up"
							}
							return x
						})
					}),
					actionButton(func() iconName { return glyphThumbDown }, "Bad reply", func() bool { return m().rating == "down" }, toneDanger, func() {
						update(core.Untracked(cx, m).id, func(x msg) msg {
							if x.rating == "down" {
								x.rating = ""
							} else {
								x.rating = "down"
							}
							return x
						})
					}),
				}}
			}},
		}}
	}

	emptyText := "A simulated assistant: every reply is scripted and streamed locally."
	if native {
		emptyText = "Chat with your local Claude or Codex CLI. Sign in to the CLI in a terminal first."
	}
	var featuredTiles []core.Node
	for i, f := range featured {
		s, _ := findScenario(f.id)
		featuredTiles = append(featuredTiles, core.View{Style: core.Style{Width: core.Pct(47), FlexGrow: 1}, Children: []core.Node{
			appear(i+1, false, press(pressProps{onPress: func() { play(s.id) }, label: s.chip},
				core.View{Style: fin.card.over(core.Style{MinHeight: core.Pt(104), Padding: core.All(12), Gap: 8, CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous,
					Background: colors.surface, BorderWidth: 1, BorderColor: colors.border}), Children: []core.Node{
					icon(f.icon, 18, colors.accent), text(s.chip, 14, semibold, colors.text), text(f.note, 12, regular, colors.muted),
				}})),
		}})
	}
	empty := func(*core.Scope) core.Node {
		return core.View{Style: column.Merge(core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(24, 16), Gap: 24}), Children: []core.Node{
			appear(0, false, core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
				iconTile(iconSpark, 56),
				text("How can I help?", 22, semibold, colors.text),
				core.Text{Content: emptyText, Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted, TextAlign: core.TextAlignCenter}},
			}}),
			core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 12}, Children: featuredTiles},
		}}
	}

	type chip struct{ id, label string }
	var chips []chip
	for _, s := range scenarios {
		chips = append(chips, chip{s.id, s.chip})
	}
	chips = append(chips, chip{"history", "Load a long conversation"})

	var menuRows []core.Node
	// choosePhoto attaches a photo from the system's picker.
	choosePhoto := func() {
		cx.PickImages(core.PickOptions{MaxDimension: 2048, Quality: 0.85}, func(files []core.PickedFile) {
			if len(files) == 0 {
				return
			}
			f := files[0]
			size := fmt.Sprintf("%d KB", max(1, (len(f.Data)+512)/1024))
			if len(f.Data) >= 1024*1024 {
				size = fmt.Sprintf("%.1f MB", float64(len(f.Data))/1024/1024)
			}
			attachment.Set(&chatAttachment{image: core.ImageSource{Data: f.Data, Size: f.Size}, file: fileRef{f.Name, size}})
		})
	}
	attachRow := func(label string, onPress func()) core.Node {
		return core.View{OnPress: func(core.PressEvent) { menuOpen.Set(false); onPress() }, Role: core.RoleButton, Label: label,
			Style:    core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Height: core.Pt(48), Padding: core.Symmetric(0, 16), Active: &core.Style{Background: alpha(colors.text, 0.06)}},
			Children: []core.Node{icon(iconAttach, 16, colors.accent), core.Text{Content: label, Style: core.Style{FlexGrow: 1, FontSize: 15, Color: colors.text}}}}
	}
	menuRows = append(menuRows,
		attachRow("Choose a photo", choosePhoto),
		attachRow("Attach a photo and a file", func() {
			attachment.Set(&chatAttachment{image: core.Src(chatPhoto), file: fileRef{"trip-notes.pdf", "2.4 MB"}})
		}))
	for _, c := range chips {
		menuRows = append(menuRows, core.View{OnPress: func(core.PressEvent) { menuOpen.Set(false); play(c.id) }, Role: core.RoleButton, Label: c.label,
			Style:    core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Height: core.Pt(48), Padding: core.Symmetric(0, 16), Active: &core.Style{Background: alpha(colors.text, 0.06)}},
			Children: []core.Node{icon(glyphRegenerate, 16, colors.muted), core.Text{Content: c.label, Style: core.Style{FlexGrow: 1, FontSize: 15, Color: colors.text}}}})
	}

	conversation := func(*core.Scope) core.Node {
		// One selection across messages: in the list, over the rows that
		// are mounted.
		return core.View{Selectable: true, Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			core.List{Ref: list, TestID: "messages", Count: func() int { return len(messages.Get()) }, Key: func(i int) any { return messages.GetUntracked()[i].id },
				EstimatedSize: 140, InitialScrollIndex: max(0, len(messages.GetUntracked())-1), Style: core.Style{FlexGrow: 1, Padding: core.Edges{Top: 8, Bottom: 16}},
				Behavior:                       core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissInteractive},
				MaintainVisibleContentPosition: &core.VisibleContentPosition{AutoscrollToBottomThreshold: 40},
				OnScrollDragStart:              func(core.ScrollEvent) { dragging = true },
				OnScroll: func(e core.ScrollEvent) {
					if dragging {
						following.Set(e.Offset.Y+e.Size.Height >= e.ContentSize.Height-40)
					}
				},
				OnMomentumEnd:   func(core.ScrollEvent) { dragging = false },
				OnScrollDragEnd: func(core.ScrollEvent) { dragging = false },
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					item := func() msg {
						ms := messages.Get()
						if i := r.Index(); i < len(ms) {
							return ms[i]
						}
						return msg{}
					}
					kind := core.NewMemo(cx, func() string { return item().kind })
					return core.View{Style: column, Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
						switch kind.Get() {
						case "date":
							return dateSeparator(func() string { return item().label })
						case "user":
							return userRow(item)
						case "assistant":
							return assistantRow(item)
						}
						return core.Group{}
					})}}
				}},
		}}
	}

	composerPlaceholder := func() string {
		if native {
			return "Message " + provider.Get()
		}
		return "Message " + string(model.Get())
	}
	submitBehavior := core.SubmitBehaviorNewline
	// Return sends on desktops, as chat apps there do (Shift+Return adds a
	// line); phones keep return for line breaks.
	if env.Desktop() {
		submitBehavior = core.SubmitBehaviorSubmit
	}
	var picker core.Node
	if native {
		picker = segmented([]option[string]{{value: "Claude", label: "Claude"}, {value: "Codex", label: "Codex"}}, provider.Get, func(v string) {
			if run == nil {
				provider.Set(v)
			}
		}, true)
	} else {
		picker = segmented([]option[chatModel]{{value: "Swift", label: "Swift"}, {value: "Deep", label: "Deep"}, {value: "Vision", label: "Vision"}}, model.Get, model.Set, true)
	}
	menuFinish := fin.menu
	menuFinish.backdrop = nil

	pane := core.View{Style: fillPage.Merge(core.Style{Background: colors.background}), Children: []core.Node{
		core.View{Style: fillPage.Merge(core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard}}), Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
				core.Show{When: func() bool { return len(messages.Get()) > 0 }, Then: conversation, Else: empty},
				// Jump to the latest message: shown while scrolled up.
				core.View{PointerEvents: core.PointerEventsBoxNone, Style: core.Style{Position: core.Absolute, Right: core.Pt(16), Bottom: core.Pt(12), Opacity: core.Alpha(0.001),
					Animated: &core.AnimatedStyle{Opacity: jump, Scale: lerp(jump, 0.7, 1)}}, Children: []core.Node{
					core.Show{When: func() bool { return !following.Get() }, Then: func(*core.Scope) core.Node {
						return roundButton(iconArrowDown, "Jump to the latest message", secondary, 0, func() { toEnd(true) })
					}},
					core.Show{When: streaming, Then: func(*core.Scope) core.Node {
						return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(2), Right: core.Pt(2), Width: core.Pt(10), Height: core.Pt(10), CornerRadius: 5,
							Background: colors.accent, BorderWidth: 2, BorderColor: colors.background}}
					}},
				}},
				// The connection banner.
				core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Left: core.Pt(16), Right: core.Pt(16), Opacity: core.Alpha(0.001),
					Animated: &core.AnimatedStyle{Opacity: bannerValue, TranslateY: lerp(bannerValue, -64, 0)}}, Children: []core.Node{
					core.Dynamic(func(*core.Scope) core.Node {
						off := bannerKind.Get() == "offline"
						t, name, title, detail := toneUp, iconCheck, "Back online", "Sending your message."
						if off {
							t, name, title, detail = toneWarn, glyphOffline, "You're offline", "Your message sends when the connection returns."
						}
						return core.View{Style: fin.menu.over(core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, MinHeight: core.Pt(48), Padding: core.Symmetric(0, 14),
							CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1, BorderColor: alpha(t.color(), 0.35)}), Children: []core.Node{
							core.View{Style: core.Style{Width: core.Pt(28), Height: core.Pt(28), CornerRadius: 14, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: t.tint()},
								Children: []core.Node{icon(name, 15, t.color())}},
							core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 1}, Children: []core.Node{text(title, 14, semibold, colors.text), text(detail, 13, regular, colors.muted)}},
						}}
					}),
				}},
				// The + menu: attachments and the scenarios, to replay them.
				core.Show{When: menuOpen.Get, Then: func(*core.Scope) core.Node {
					return core.View{OnPress: func(core.PressEvent) { menuOpen.Set(false) }, NoFocus: true, Label: "Close the menu",
						Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}}
				}},
				core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0),
					Background: black(0.45), Opacity: core.Alpha(0.001), Animated: &core.AnimatedStyle{Opacity: menuValue}}},
				core.View{StyleFunc: func() core.Style {
					s := core.Style{Position: core.Absolute, Left: core.Pt(16), Right: core.Pt(16), Bottom: core.Pt(8), Opacity: core.Alpha(0.001),
						Animated: &core.AnimatedStyle{Opacity: menuValue, TranslateY: lerp(menuValue, 24, 0), Scale: lerp(menuValue, 0.97, 1)}}
					if !menuOpen.Get() {
						s.Visibility = core.VisibilityHidden
					}
					return s
				}, Children: []core.Node{core.View{Style: fin.menu.over(core.Style{CornerRadius: radiusCard, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1,
					Padding: core.Symmetric(6, 0), MaxHeight: core.Pt(460), Overflow: core.Hidden}), Children: []core.Node{
					core.Text{Content: "ADD AND REPLAY", Style: core.Style{FontSize: 12, FontWeight: semibold, Color: colors.faint, Padding: core.Edges{Top: 8, Right: 16, Bottom: 4, Left: 16}}},
					core.Scroll{Style: core.Style{FlexShrink: 1}, Children: menuRows},
				}}}},
			}},
			// Suggestions while the conversation is empty.
			core.Show{When: func() bool { return len(messages.Get()) == 0 }, Then: func(*core.Scope) core.Node {
				var nodes []core.Node
				for _, c := range chips {
					nodes = append(nodes, press(pressProps{onPress: func() { play(c.id) }, hitSlop: 4, label: c.label},
						core.View{Style: fin.pill.over(core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 14), CornerRadius: 18, JustifyContent: core.JustifyCenter, BorderWidth: 1}),
							Children: []core.Node{text(c.label, 13, medium, colors.text)}}))
				}
				return core.Scroll{Horizontal: true, HideIndicator: true, Behavior: core.ScrollBehavior{KeyboardShouldPersistTaps: core.KeyboardTapsHandled}, Style: core.Style{MinHeight: core.Pt(52)},
					Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(8, 16)}, Children: nodes}}}
			}},
			// The composer.
			core.View{StyleFunc: func() core.Style {
				bottom := core.Points(8)
				if env.KeyboardHeight() == 0 {
					bottom += env.SafeArea().Bottom
				}
				return column.Merge(core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: bottom, Left: 16}, Gap: 8})
			}, Children: []core.Node{
				core.Dynamic(func(*core.Scope) core.Node {
					a := attachment.Get()
					if a == nil {
						return nil
					}
					return appear(0, false, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
						attachedPhoto(a.image, 48, 48),
						core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignStart}, Children: []core.Node{fileChip(a.file, func() { attachment.Set(nil) })}},
					}})
				}),
				// Glass like the menu above it, without the blur: nothing
				// scrolls under it.
				core.View{Style: menuFinish.over(core.Style{CornerRadius: 24, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1, Padding: core.All(6), Gap: 4}), Children: []core.Node{
					base.Input{Value: draft.Get, OnChange: draft.Set, Multiline: true, SubmitBehavior: submitBehavior, TestID: "composer",
						OnSubmit: func(string) {
							if !core.Untracked(cx, streaming) {
								send()
							}
						},
						Placeholder: core.Untracked(cx, composerPlaceholder), OnFocus: func() { menuOpen.Set(false) },
						OnContentSizeChange: func(s core.Size) { inputHeight.Set(min(100, max(20, s.Height))) },
						StyleFunc: func() core.Style {
							return core.Style{PlaceholderColor: colors.faint, SelectionColor: colors.accent, Height: core.Pt(inputHeight.Get() + 20), FontSize: 15, LineHeight: 20,
								Color: colors.text, Padding: core.Symmetric(10, 10)}
						}},
					core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
						core.View{StyleFunc: func() core.Style {
							s := core.Style{Transition: core.Transition{Duration: ms(200), Easing: core.EaseOut, Properties: core.TransitionTransform}}
							if menuOpen.Get() {
								s.Transform.Rotate = 45
							}
							return s
						}, Children: []core.Node{roundButton(iconPlus, "Add", secondary, 0, func() { toggled(menuOpen) })}},
						core.View{Style: fill, Children: []core.Node{picker}},
						core.Dynamic(func(*core.Scope) core.Node {
							if streaming() {
								return press(pressProps{onPress: stop, scale: 0.9, label: "Stop generating"},
									core.View{Style: fin.light.over(core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 22, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
										Background: colors.light}), Children: []core.Node{core.View{Style: core.Style{Width: core.Pt(13), Height: core.Pt(13), CornerRadius: 3, Background: colors.onLight}}}})
							}
							return core.View{StyleFunc: func() core.Style {
								if canSend() {
									return core.Style{}
								}
								return core.Style{Opacity: core.Alpha(0.35)}
							}, Children: []core.Node{press(pressProps{onPress: send, disabled: func() bool { return !canSend() }, scale: 0.92, label: "Send"},
								core.View{Style: fin.light.over(core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 22, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
									Background: colors.light}), Children: []core.Node{icon(iconArrowUp, 18, colors.onLight)}})}}
						}),
					}},
				}},
			}},
		}},
	}}
	if !wide {
		return pane
	}

	// Desktop: the scenarios as conversations, each opened afresh.
	current := core.NewSignal(cx, "")
	newChat := func() {
		stop()
		cx.Batch(func() {
			messages.Set(nil)
			attachment.Set(nil)
			menuOpen.Set(false)
			current.Set("")
		})
	}
	openChat := func(id string) {
		newChat()
		current.Set(id)
		play(id)
	}
	var convs []core.Node
	for _, c := range chips {
		sel := func() bool { return current.Get() == c.id }
		detail := "Weeks of messages, loaded at once"
		if s, ok := findScenario(c.id); ok {
			detail = s.prompt
		}
		convs = append(convs, core.View{OnPress: func(core.PressEvent) { openChat(c.id) }, Role: core.RoleButton, Label: c.label, Selected: sel,
			Style: core.Style{Padding: core.Symmetric(9, 12), CornerRadius: radiusControl, Gap: 2, Cursor: core.CursorPointer,
				Hover: &core.Style{Background: colors.surface}, Active: &core.Style{Background: colors.pressed}, Selected: &core.Style{Background: colors.pressed}},
			Children: []core.Node{
				core.Text{Content: c.label, MaxLines: 1, StyleFunc: func() core.Style {
					w := regular
					if sel() {
						w = semibold
					}
					return core.Style{FontSize: 14, FontWeight: w, Color: colors.text}
				}},
				core.Text{Content: detail, MaxLines: 1, Style: core.Style{FontSize: 12, Color: colors.muted}},
			}})
	}
	return core.View{Style: fillPage.Merge(core.Style{Direction: core.Row, Background: colors.background}), Children: []core.Node{
		core.Scroll{Style: core.Style{Width: core.Pt(280), MinWidth: core.Pt(280), Borders: core.Edges{Right: 1}, BorderColors: core.EdgeColors{Right: colors.line}}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(12), Gap: 2}, Children: append([]core.Node{
				press(pressProps{onPress: newChat, label: "New chat"}, core.View{Style: fin.raised.over(core.Style{Height: core.Pt(40), CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous,
					Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 8, Background: colors.raised, BorderWidth: 1, BorderColor: colors.border, Cursor: core.CursorPointer}),
					Children: []core.Node{icon(iconPlus, 15, colors.text), text("New chat", 14, semibold, colors.text)}}),
				core.Text{Content: "CONVERSATIONS", Style: core.Style{FontSize: 11, FontWeight: semibold, Color: colors.faint, LetterSpacing: 0.6, Padding: core.Edges{Top: 18, Right: 12, Bottom: 6, Left: 12}}},
			}, convs...)},
		}},
		pane,
	}}
}
