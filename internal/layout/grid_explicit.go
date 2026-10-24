package layout

import "math"

// Initialising grid tracks from styles: the explicit grid's size, including
// auto-repeated tracks, and the tracks themselves.

// autoRepeatStrategy says how many auto-fill or auto-fit repetitions to use.
type autoRepeatStrategy uint8

const (
	// maxRepetitionsThatDoNotOverflow fills a definite size or max size.
	maxRepetitionsThatDoNotOverflow autoRepeatStrategy = iota
	// minRepetitionsThatDoOverflow fills a definite min size.
	minRepetitionsThatDoOverflow
)

// saturatingU32 converts a float to uint32 as Rust's `as` cast does:
// saturating, NaN being 0.
func saturatingU32(f float32) uint32 {
	switch {
	case f != f || f <= 0:
		return 0
	case f >= math.MaxUint32:
		return math.MaxUint32
	}
	return uint32(f)
}

func saturatingAddU32(a, b uint32) uint32 {
	if a > math.MaxUint32-b {
		return math.MaxUint32
	}
	return a + b
}

// gridTrackDefiniteValue is the fixed size of a track sizing function in an
// auto-repeat computation: its max, at least its min.
func gridTrackDefiniteValue(t TrackSizing, parentSize Opt) float32 {
	maxSize := t.Max.definiteValue(parentSize)
	minSize := t.Min.definiteValue(parentSize)
	if maxSize.Valid {
		return fmaxOpt(maxSize.Value, minSize)
	}
	return minSize.Or(0)
}

// explicitGridSizeInAxis returns the number of auto repetitions and the
// number of tracks of the explicit grid's template on one axis.
func explicitGridSizeInAxis(style *Style, autoFitContainerSize Opt, strategy autoRepeatStrategy,
	axis AbsoluteAxis) (autoRepetitions, trackCount uint16) {
	template := style.gridTemplateTracks(axis)

	// A template with no tracks has no explicit tracks.
	if len(template) == 0 {
		return 0, 0
	}

	// A repetition with no tracks invalidates the whole definition, which
	// then gives no explicit tracks.
	for _, c := range template {
		if c.Repeat != nil && c.Repeat.trackCount() == 0 {
			return 0, 0
		}
	}

	// The number of tracks from single tracks and repetitions with a fixed
	// count (clamped to the track limit to prevent overflow).
	var nonAutoCount32 uint32
	for _, c := range template {
		add := uint32(1)
		if c.Repeat != nil {
			add = 0
			if !c.Repeat.Count.isAuto() {
				add = uint32(uint16(c.Repeat.Count)) * uint32(c.Repeat.trackCount())
			}
		}
		nonAutoCount32 = saturatingAddU32(nonAutoCount32, add)
	}
	nonAutoRepeatingTrackCount := uint16(min(nonAutoCount32, maxGridTracks))

	autoRepetitionCount := 0
	allHaveFixedComponent := true
	for _, c := range template {
		if c.isAutoRepetition() {
			autoRepetitionCount++
		}
		if c.Repeat == nil {
			allHaveFixedComponent = allHaveFixedComponent && c.Track.hasFixedComponent()
		} else {
			for _, t := range c.Repeat.Tracks {
				allHaveFixedComponent = allHaveFixedComponent && t.hasFixedComponent()
			}
		}
	}

	// A template with several auto repetitions, or one combined with
	// non-fixed track sizing functions, is invalid and disregarded entirely.
	if !(autoRepetitionCount == 0 || (autoRepetitionCount == 1 && allHaveFixedComponent)) {
		return 0, 0
	}

	// Without repetitions, the number of explicit tracks is the number of
	// track definitions.
	if autoRepetitionCount == 0 {
		return 0, nonAutoRepeatingTrackCount
	}

	var insertionPoint uint32
	var repetition *Repetition
	for _, c := range template {
		if c.Repeat == nil {
			insertionPoint = saturatingAddU32(insertionPoint, 1)
			continue
		}
		if c.Repeat.Count.isAuto() {
			repetition = c.Repeat
			break
		}
		insertionPoint = saturatingAddU32(insertionPoint, uint32(uint16(c.Repeat.Count))*uint32(c.Repeat.trackCount()))
	}
	repetitionTrackCount := repetition.trackCount()

	// Determine the number of repetitions.
	numRepetitions := uint32(1)
	if autoFitContainerSize.Valid {
		innerContainerSize := autoFitContainerSize.Value
		parentSize := autoFitContainerSize

		var nonRepeatingUsed float32
		for _, c := range template {
			switch {
			case c.Repeat == nil:
				nonRepeatingUsed += gridTrackDefiniteValue(c.Track, parentSize)
			case !c.Repeat.Count.isAuto():
				var sum float32
				for _, t := range c.Repeat.Tracks {
					sum += gridTrackDefiniteValue(t, parentSize)
				}
				nonRepeatingUsed += sum * float32(uint16(c.Repeat.Count))
			default:
				nonRepeatingUsed += 0
			}
		}
		gapSize := style.gap().get(axis).resolveOrZero(Some(innerContainerSize))

		// The space a single repetition of the repeated tracks takes.
		var perRepetitionTrackUsed float32
		for _, t := range repetition.Tracks {
			perRepetitionTrackUsed += gridTrackDefiniteValue(t, parentSize)
		}

		// The first repetition is special-cased because its number of gaps
		// depends on the number of non-repeating tracks.
		gaps := uint32(nonAutoRepeatingTrackCount) + uint32(repetitionTrackCount)
		if gaps > 0 {
			gaps--
		}
		firstAndNonRepeatingUsed := nonRepeatingUsed + perRepetitionTrackUsed + float32(float32(gaps)*gapSize)

		// If a single repetition already overflows, the count is 1 (the number
		// of repetitions is floored at 1).
		if firstAndNonRepeatingUsed <= innerContainerSize {
			perRepetitionGapUsed := float32(float32(repetitionTrackCount) * gapSize)
			perRepetitionUsed := perRepetitionTrackUsed + perRepetitionGapUsed
			fit := (innerContainerSize - firstAndNonRepeatingUsed) / perRepetitionUsed

			// For a preferred or max size, the most repetitions that fit
			// without overflowing; for a min size, the fewest that overflow.
			// Either way plus the repetition accounted for above. Conversions
			// saturate, so a huge or infinite count (as from zero-sized
			// tracks) is clamped below.
			if strategy == maxRepetitionsThatDoNotOverflow {
				numRepetitions = saturatingAddU32(saturatingU32(floor32(fit)), 1)
			} else {
				numRepetitions = saturatingAddU32(saturatingU32(ceil32(fit)), 1)
			}
		}
	}

	// Clamp the number of repetitions so that tracks before the auto-repeat
	// are kept and tracks after it are truncated in source order
	// (https://www.w3.org/TR/css-grid-1/#overlarge-grids).
	var remaining uint32
	if insertionPoint < maxGridTracks {
		remaining = maxGridTracks - insertionPoint
	}
	var repetitions uint16
	if remaining > 0 {
		maxRepetitions := (remaining + uint32(repetitionTrackCount) - 1) / uint32(repetitionTrackCount)
		repetitions = uint16(min(max(numRepetitions, 1), maxRepetitions))
	}

	trackCount = uint16(min(uint32(nonAutoRepeatingTrackCount)+uint32(repetitionTrackCount)*uint32(repetitions),
		maxGridTracks))
	return repetitions, trackCount
}

// initializeGridTracks creates the tracks and gutters of one axis.
func initializeGridTracks(tracks []gridTrack, counts trackCounts, style *Style, axis AbsoluteAxis,
	autoRepetitionCount uint16, trackHasItems func(int) bool) []gridTrack {
	template := style.gridTemplateTracks(axis)
	autoTracks := style.gridAutoTracks(axis)
	gap := style.gap().get(axis)

	tracks = tracks[:0]
	if need := counts.len()*2 + 1; cap(tracks) < need {
		tracks = make([]gridTrack, 0, need)
	}
	tracks = append(tracks, newGridGutter(gap))

	autoTrackCount := len(autoTracks)

	// Negative implicit tracks.
	if counts.negativeImplicit > 0 {
		if autoTrackCount == 0 {
			tracks = createImplicitTracks(tracks, counts.negativeImplicit, nil, 0, gap)
		} else {
			offset := autoTrackCount - int(counts.negativeImplicit)%autoTrackCount
			tracks = createImplicitTracks(tracks, counts.negativeImplicit, autoTracks, offset, gap)
		}
	}

	currentTrackIndex := int(counts.negativeImplicit)

	// The explicit track count is clamped to the track limit, so track
	// creation from the template must be truncated to match.
	explicitTrackLimit := int(counts.negativeImplicit) + int(counts.explicit)

	// Explicit tracks. The count is checked (rather than relying on an empty
	// template) because a count of zero can come from an invalid template,
	// which must be ignored.
	if counts.explicit > 0 {
		for _, c := range template {
			switch {
			case c.Repeat == nil:
				if currentTrackIndex < explicitTrackLimit {
					tracks = append(tracks, newGridTrack(c.Track.Min, c.Track.Max), newGridGutter(gap))
					currentTrackIndex++
				}
			case !c.Repeat.Count.isAuto():
				r := c.Repeat
				n := min(int(r.trackCount())*int(uint16(r.Count)), explicitTrackLimit-currentTrackIndex)
				for i := range n {
					t := r.Tracks[i%len(r.Tracks)]
					tracks = append(tracks, newGridTrack(t.Min, t.Max), newGridGutter(gap))
					currentTrackIndex++
				}
			default:
				r := c.Repeat
				n := min(int(r.trackCount())*int(autoRepetitionCount), explicitTrackLimit-currentTrackIndex)
				for i := range n {
					t := r.Tracks[i%len(r.Tracks)]
					track, gutter := newGridTrack(t.Min, t.Max), newGridGutter(gap)
					// Auto-fit tracks without items collapse.
					if r.Count == AutoFit && !trackHasItems(currentTrackIndex) {
						track.collapse()
						gutter.collapse()
					}
					tracks = append(tracks, track, gutter)
					currentTrackIndex++
				}

				// When collapsing auto-fit tracks (above), the gutter after
				// collapsed tracks collapses but not the one before. That is
				// right as long as a non-collapsed track follows somewhere in
				// the track list. If the auto-fit repeat is at the very end of
				// the list, none does: then iterate backwards to the first
				// non-collapsed track and collapse the gutter just after it.
				// Everything found on the way is already collapsed, so this
				// collapses all tracks and gutters until that track.
				if r.Count == AutoFit && currentTrackIndex == counts.len() {
					for i := len(tracks) - 1; i >= 0; i-- {
						if !tracks[i].isGutter && !tracks[i].isCollapsed {
							break
						}
						tracks[i].collapse()
					}
				}
			}
		}
	}

	gridAreaTracks := uint16(int(counts.negativeImplicit) + int(counts.explicit) - currentTrackIndex)

	// Positive implicit tracks.
	tracks = createImplicitTracks(tracks, counts.positiveImplicit+gridAreaTracks, autoTracks, 0, gap)

	// The first and last grid lines are collapsed.
	tracks[0].collapse()
	tracks[len(tracks)-1].collapse()
	return tracks
}

// createImplicitTracks appends count tracks sized by autoTracks, cycling
// from offset, or auto when there are none.
func createImplicitTracks(tracks []gridTrack, count uint16, autoTracks []TrackSizing, offset int, gap Length) []gridTrack {
	for i := range int(count) {
		t := TrackSizing{Min: Auto, Max: Auto}
		if len(autoTracks) > 0 {
			t = autoTracks[(offset+i)%len(autoTracks)]
		}
		tracks = append(tracks, newGridTrack(t.Min, t.Max), newGridGutter(gap))
	}
	return tracks
}
