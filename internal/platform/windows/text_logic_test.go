package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestLimitedMeasureWidth(t *testing.T) {
	tests := []struct {
		name string
		in   limitedMeasureInput
		want float32
	}{
		{
			name: "tail uses trimmed width when DirectWrite trims at max height",
			in: limitedMeasureInput{
				preWidth: 55, trimmedWidth: 70, trimmedLines: 2, shown: 2,
				limit: 72, bounded: true, softWrap: true, ellipsize: driver.EllipsizeTail,
			},
			want: 70,
		},
		{
			name: "tail keeps preWidth when trimmed width is narrower so Layout does not rewrap earlier",
			in: limitedMeasureInput{
				preWidth: 39, trimmedWidth: 37, trimmedLines: 2, shown: 2,
				limit: 40, bounded: true, softWrap: true, ellipsize: driver.EllipsizeTail,
			},
			want: 39,
		},
		{
			name: "tail clamps trimmed width to limit",
			in: limitedMeasureInput{
				preWidth: 55, trimmedWidth: 80, trimmedLines: 2, shown: 2,
				limit: 72, bounded: true, softWrap: true, ellipsize: driver.EllipsizeTail,
			},
			want: 72,
		},
		{
			name: "tail keeps available width on soft wrap when GetMetrics still reports hidden lines",
			in: limitedMeasureInput{
				preWidth: 55, trimmedWidth: 140, trimmedLines: 3, shown: 2,
				limit: 72, bounded: true, softWrap: true, ellipsize: driver.EllipsizeTail,
			},
			want: 72,
		},
		{
			name: "tail hard break keeps trimmed width without expanding to room",
			in: limitedMeasureInput{
				preWidth: 30, trimmedWidth: 38, trimmedLines: 1, shown: 1,
				limit: 200, bounded: true, softWrap: false, ellipsize: driver.EllipsizeTail,
			},
			want: 38,
		},
		{
			name: "tail hard break ignores hidden line width when GetMetrics still counts hidden lines",
			in: limitedMeasureInput{
				preWidth: 30, trimmedWidth: 180, trimmedLines: 2, shown: 1,
				limit: 200, bounded: true, softWrap: false, ellipsize: driver.EllipsizeTail,
			},
			want: 30,
		},
		{
			name: "tail hard break unbounded matches bounded with room",
			in: limitedMeasureInput{
				preWidth: 30, trimmedWidth: 38, trimmedLines: 1, shown: 1,
				bounded: false, softWrap: false, ellipsize: driver.EllipsizeTail,
			},
			want: 38,
		},
		{
			name: "clip keeps pre-trim width even on soft wrap",
			in: limitedMeasureInput{
				preWidth: 55, trimmedWidth: 70, trimmedLines: 2, shown: 2,
				limit: 72, bounded: true, softWrap: true, ellipsize: driver.EllipsizeClip,
			},
			want: 55,
		},
		{
			name: "head/middle soft wrap keeps full available width for CutRest",
			in: limitedMeasureInput{
				preWidth: 55, trimmedWidth: 60, trimmedLines: 2, shown: 2,
				limit: 72, bounded: true, softWrap: true, ellipsize: driver.EllipsizeMiddle, useCut: true,
			},
			want: 72,
		},
		{
			name: "head/middle hard break keeps intrinsic width with room to spare",
			in: limitedMeasureInput{
				preWidth: 30, trimmedWidth: 38, trimmedLines: 1, shown: 1,
				limit: 200, bounded: true, softWrap: false, ellipsize: driver.EllipsizeHead, useCut: true,
			},
			want: 38,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := limitedMeasureWidth(tc.in); got != tc.want {
				t.Errorf("limitedMeasureWidth(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSinglePassMeasureWidth(t *testing.T) {
	if got := singlePassMeasureWidth(66, 72, true, true, true); got != 72 {
		t.Errorf("trimmed head/middle single line = %v, want full limit 72", got)
	}
	if got := singlePassMeasureWidth(30, 72, true, true, false); got != 30 {
		t.Errorf("untrimmed head/middle single line = %v, want 30", got)
	}
	if got := singlePassMeasureWidth(66, 72, true, false, true); got != 66 {
		t.Errorf("trimmed tail single line = %v, want trimmed width 66", got)
	}
	if got := singlePassMeasureWidth(130, 72, true, false, false); got != 72 {
		t.Errorf("clipped overflowing line = %v, want limit 72", got)
	}
	if got := singlePassMeasureWidth(130, 0, false, true, true); got != 130 {
		t.Errorf("unbounded line = %v, want 130", got)
	}
}
