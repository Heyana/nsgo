package driver

import (
	"math"
	"time"
)

const (
	defaultSpringResponse = 500 * time.Millisecond
	minSpringResponse     = time.Millisecond
	minSpringDampingRatio = 0.05
	maxSpringSettle       = 60 * time.Second
	springUnitSettleEps   = 1e-3
	springFrameSettleEps  = 0.05 // points; sub-pixel even at 3x scale
	springCritTol         = 1e-5
)

// springParams resolves a LayoutSpring's natural period (in seconds) and damping ratio (zeta). When Response > 0 it is used directly; otherwise a positive Duration is mapped to the physical response whose settling envelope matches Duration; when neither is set, Response defaults to 500 ms. DampingRatio takes precedence over Damping, defaulting to 1 (critically damped).
func (tr *LayoutTransition) springParams() (responseSec, zeta float64) {
	zeta = tr.DampingRatio
	if zeta <= 0 && tr.Damping > 0 {
		zeta = float64(tr.Damping)
	}
	if zeta <= 0 {
		zeta = 1
	}
	if zeta < minSpringDampingRatio {
		zeta = minSpringDampingRatio
	}
	var resp time.Duration
	switch {
	case tr.Response > 0:
		resp = tr.Response
	case tr.Duration > 0:
		const ln1000 = 6.907755278982137
		const critU = 9.23342
		dur := tr.Duration.Seconds()
		var w0 float64
		switch {
		case zeta < 1-springCritTol:
			w0 = (ln1000 / zeta) / dur
		case zeta <= 1+springCritTol:
			w0 = critU / dur
		default:
			s := math.Sqrt(zeta*zeta - 1)
			c1 := (zeta + s) / (2 * s)
			w0 = math.Log(c1/springUnitSettleEps) * (zeta + s) / dur
		}
		resp = time.Duration(2 * math.Pi / w0 * float64(time.Second))
	default:
		resp = defaultSpringResponse
	}
	if resp < minSpringResponse {
		resp = minSpringResponse
	}
	return resp.Seconds(), zeta
}

// evalSpring1D evaluates the exact analytical solution of a 1D damped harmonic oscillator y” + 2*zeta*w0*y' + w0^2*y = 0 at time t >= 0 (seconds), where y = x - target, starting from displacement y0 and velocity v0 (units per second). It returns y(t), v(t) and whether both |y| <= eps and |v|/w0 <= eps (an energy bound guaranteeing future displacement stays near rest).
func evalSpring1D(y0, v0, w0, zeta, t, eps float64) (y, v float64, settled bool) {
	if t <= 0 {
		if math.Abs(y0) <= eps && math.Abs(v0)/w0 <= eps {
			return 0, 0, true
		}
		return y0, v0, false
	}
	if t >= maxSpringSettle.Seconds() {
		return 0, 0, true
	}
	switch {
	case zeta < 1-springCritTol:
		alpha := zeta * w0
		wd := w0 * math.Sqrt(1-zeta*zeta)
		env := math.Exp(-alpha * t)
		cos, sin := math.Cos(wd*t), math.Sin(wd*t)
		b := (v0 + alpha*y0) / wd
		y = env * (y0*cos + b*sin)
		v = env * (v0*cos - (alpha*v0+w0*w0*y0)/wd*sin)
	case zeta <= 1+springCritTol:
		env := math.Exp(-w0 * t)
		b := v0 + w0*y0
		y = env * (y0 + b*t)
		v = env * (v0 - w0*b*t)
	default:
		s := math.Sqrt(zeta*zeta - 1)
		gamma := w0 * s
		r2 := -w0 * (zeta + s)
		r1 := -w0 / (zeta + s)
		c1 := (v0 - r2*y0) / (2 * gamma)
		c2 := y0 - c1
		e1, e2 := math.Exp(r1*t), math.Exp(r2*t)
		y = c1*e1 + c2*e2
		v = c1*r1*e1 + c2*r2*e2
	}
	if math.Abs(y) <= eps && math.Abs(v)/w0 <= eps {
		return 0, 0, true
	}
	return y, v, false
}

// springSettleDuration returns the time it takes a unit step from rest to settle under tr's spring physics.
func (tr *LayoutTransition) springSettleDuration() time.Duration {
	respSec, zeta := tr.springParams()
	w0 := 2 * math.Pi / respSec
	var secs float64
	switch {
	case zeta < 1-springCritTol:
		u1 := -math.Log(springUnitSettleEps * math.Sqrt(1-zeta*zeta))
		const u2 = 9.23342
		secs = min(u1, u2) / (zeta * w0)
	case zeta <= 1+springCritTol:
		const critU = 9.23342
		secs = critU / w0
	default:
		s := math.Sqrt(zeta*zeta - 1)
		r1 := -w0 / (zeta + s)
		c1 := (zeta + s) / (2 * s)
		hi := min(math.Log(c1/springUnitSettleEps)/(-r1), maxSpringSettle.Seconds())
		lo := 0.0
		for range 32 {
			mid := (lo + hi) / 2
			if _, _, ok := evalSpring1D(-1, 0, w0, zeta, mid, springUnitSettleEps); ok {
				hi = mid
			} else {
				lo = mid
			}
		}
		secs = hi
	}
	secs = min(max(secs, 0), maxSpringSettle.Seconds())
	return time.Duration(math.Ceil(secs * float64(time.Second)))
}

// LayoutMotion tracks a view's frame moving under a LayoutTransition. For LayoutSpring, all four frame components (x, y, width, height) evolve independently by exact spring physics until they settle, and Retarget preserves the last frame's position and velocity so mid-flight changes flow smoothly without snapping or restarting from zero velocity.
type LayoutMotion struct {
	tr    LayoutTransition
	start time.Duration
	from  [4]float64
	vel0  [4]float64
	to    [4]float64
	pos   [4]float64
	vel   [4]float64
	shown Bounds
	done  bool
}

// StartLayoutMotion starts a frame motion from from to to under tr at clock time now.
func StartLayoutMotion(from, to Bounds, tr LayoutTransition, now time.Duration) LayoutMotion {
	fv, tv := boundsToVec(from), boundsToVec(to)
	m := LayoutMotion{
		tr:    tr,
		start: now,
		from:  fv,
		to:    tv,
		pos:   fv,
		shown: from,
		done:  from == to,
	}
	return m
}

// Retarget redirects m to a new target frame to under tr at clock time now. It first advances m to now so the new motion begins from m's exact current position and (when tr is LayoutSpring) current velocity.
func (m *LayoutMotion) Retarget(to Bounds, tr LayoutTransition, now time.Duration) {
	if !m.done {
		m.Step(now)
	}
	m.from = m.pos
	if tr.Curve == LayoutSpring {
		m.vel0 = m.vel
	} else {
		m.vel0 = [4]float64{}
		m.vel = [4]float64{}
	}
	m.to = boundsToVec(to)
	m.tr = tr
	m.start = now
	m.shown = vecToBounds(m.from)
	m.done = false
	if tr.Curve == LayoutSpring {
		respSec, _ := tr.springParams()
		w0 := 2 * math.Pi / respSec
		settled := true
		for i := range 4 {
			if math.Abs(m.from[i]-m.to[i]) > springFrameSettleEps || math.Abs(m.vel0[i])/w0 > springFrameSettleEps {
				settled = false
				break
			}
		}
		if settled {
			m.pos = m.to
			m.vel = [4]float64{}
			m.shown = to
			m.done = true
		}
	} else if m.from == m.to {
		m.shown = to
		m.done = true
	}
}

// Step advances m to clock time now, returning the shown frame and whether the motion has finished.
func (m *LayoutMotion) Step(now time.Duration) (Bounds, bool) {
	if m.done {
		return m.shown, true
	}
	elapsed := now - m.start
	if m.tr.Curve == LayoutSpring {
		if elapsed <= m.tr.Delay {
			m.pos = m.from
			m.vel = m.vel0
			m.shown = vecToBounds(m.from)
			return m.shown, false
		}
		t := (elapsed - m.tr.Delay).Seconds()
		respSec, zeta := m.tr.springParams()
		w0 := 2 * math.Pi / respSec
		allSettled := true
		for i := range 4 {
			y, v, ok := evalSpring1D(m.from[i]-m.to[i], m.vel0[i], w0, zeta, t, springFrameSettleEps)
			m.pos[i] = m.to[i] + y
			m.vel[i] = v
			if !ok {
				allSettled = false
			}
		}
		if allSettled {
			m.pos = m.to
			m.vel = [4]float64{}
			m.shown = vecToBounds(m.to)
			m.done = true
			return m.shown, true
		}
		m.shown = vecToBounds(m.pos)
		return m.shown, false
	}
	p, done := m.tr.At(elapsed)
	m.vel = [4]float64{}
	if done {
		m.pos = m.to
		m.shown = vecToBounds(m.to)
		m.done = true
		return m.shown, true
	}
	for i := range 4 {
		m.pos[i] = m.from[i] + (m.to[i]-m.from[i])*p
	}
	m.shown = vecToBounds(m.pos)
	return m.shown, false
}

// Shown is the frame m last evaluated.
func (m *LayoutMotion) Shown() Bounds { return m.shown }

// From is the frame m started from.
func (m *LayoutMotion) From() Bounds { return vecToBounds(m.from) }

// Target is the frame m moves toward.
func (m *LayoutMotion) Target() Bounds { return vecToBounds(m.to) }

// Transition is the LayoutTransition driving m.
func (m *LayoutMotion) Transition() LayoutTransition { return m.tr }

// Velocity is the last evaluated velocity (x, y, width, height) in points per second.
func (m *LayoutMotion) Velocity() [4]float64 { return m.vel }

// Done reports whether m has settled at its target.
func (m *LayoutMotion) Done() bool { return m.done }

func boundsToVec(b Bounds) [4]float64 {
	return [4]float64{float64(b.Origin.X), float64(b.Origin.Y), float64(b.Size.Width), float64(b.Size.Height)}
}

func vecToBounds(v [4]float64) Bounds {
	return Bounds{
		Origin: Point{X: Points(v[0]), Y: Points(v[1])},
		Size:   Size{Width: max(Points(v[2]), 0), Height: max(Points(v[3]), 0)},
	}
}
