// Paint carried by a text run, in points or percentages of each font box.
export interface SpanBackgroundPaint {
  c: string;
  r?: number;
  t?: number;
  b?: number;
  tp?: boolean;
  bp?: boolean;
}

export interface SpanBand {
  x: number;
  y: number;
  width: number;
  height: number;
  radius: number;
  color: string;
}

export interface SpanRange {
  start: number;
  end: number;
}

export interface SpanRectangle {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

// One native line may contain several font runs, including emoji fallback. Join touching boxes before rounding so their internal edges stay square.
export function joinSpanBoxes(rects: SpanRectangle[]): SpanRectangle[] {
  const sorted = rects.slice().sort((a: SpanRectangle, b: SpanRectangle) => a.left - b.left);
  const out: SpanRectangle[] = [];
  sorted.forEach((rect: SpanRectangle) => {
    const last = out.length > 0 ? out[out.length - 1] : undefined;
    if (last !== undefined && rect.left <= last.right + 0.01 && rect.top < last.bottom && rect.bottom > last.top) {
      last.left = Math.min(last.left, rect.left);
      last.top = Math.min(last.top, rect.top);
      last.right = Math.max(last.right, rect.right);
      last.bottom = Math.max(last.bottom, rect.bottom);
    } else {
      out.push({ left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom });
    }
  });
  return out;
}

// spanLineRange clips a run to the visible characters of one native line. JavaScript and ArkUI both address the text with UTF-16 offsets.
export function spanLineRange(value: string, lineStart: number, lineEnd: number,
  runStart: number, runEnd: number): SpanRange | undefined {
  let end = Math.min(value.length, lineEnd);
  while (end > lineStart && /\s/.test(value.charAt(end - 1))) {
    end--;
  }
  const start = Math.max(lineStart, runStart);
  end = Math.min(end, runEnd);
  return end > start ? { start: start, end: end } : undefined;
}

// backgroundBand resolves insets after native layout supplied a font box. It changes only paint, leaving the text's dimensions and selection alone.
export function backgroundBand(left: number, top: number, right: number, bottom: number,
  paint: SpanBackgroundPaint): SpanBand | undefined {
  const fullHeight = bottom - top;
  const topInset = Math.min(fullHeight, Math.max(0, (paint.t ?? 0) * (paint.tp ? fullHeight / 100 : 1)));
  const bottomInset = Math.min(fullHeight, Math.max(0, (paint.b ?? 0) * (paint.bp ? fullHeight / 100 : 1)));
  const height = fullHeight - topInset - bottomInset;
  const width = right - left;
  if (width <= 0 || height <= 0) {
    return undefined;
  }
  return { x: left, y: top + topInset, width: width, height: height,
    radius: Math.max(0, Math.min(paint.r ?? 0, width / 2, height / 2)), color: paint.c };
}
