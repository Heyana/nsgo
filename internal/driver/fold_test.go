package driver

import (
	"encoding/json"
	"reflect"
	"testing"
)

func rect(x, y, w, h Points) Bounds {
	return Bounds{Origin: Point{X: x, Y: y}, Size: Size{Width: w, Height: h}}
}

func TestDivide(t *testing.T) {
	size := Size{Width: 800, Height: 600}
	hinge := Obstruction{Frame: rect(390, 0, 20, 600), Occluding: true, Separating: true, State: HingeOpen}
	halfOpen := Obstruction{Frame: rect(0, 300, 800, 0), Horizontal: true, Separating: true, State: HingeHalfOpen}
	flat := Obstruction{Frame: rect(400, 0, 0, 600), State: HingeOpen}
	covered := Obstruction{Frame: rect(390, 0, 20, 600), Occluding: true, State: HingeOpen}
	cases := []struct {
		name         string
		obstructions []Obstruction
		want         Geometry
	}{
		{"no fold", nil, Geometry{Regions: []Region{{Frame: rect(0, 0, 800, 600)}}}},
		{"an open hinge between two panels", []Obstruction{hinge}, Geometry{
			Regions:      []Region{{Frame: rect(0, 0, 390, 600)}, {Frame: rect(410, 0, 390, 600)}},
			Obstructions: []Obstruction{hinge},
		}},
		{"a half-open fold", []Obstruction{halfOpen}, Geometry{
			Regions:      []Region{{Frame: rect(0, 0, 800, 300)}, {Frame: rect(0, 300, 800, 300)}},
			Obstructions: []Obstruction{halfOpen},
		}},
		{"a flexible screen opened flat", []Obstruction{flat}, Geometry{
			Regions:      []Region{{Frame: rect(0, 0, 800, 600)}},
			Obstructions: []Obstruction{flat},
		}},
		{"an occluding fold that does not separate", []Obstruction{covered}, Geometry{
			Regions:      []Region{{Frame: rect(0, 0, 800, 600), Reserved: rect(390, 0, 20, 600)}},
			Obstructions: []Obstruction{covered},
		}},
		{"a frame outside the surface", []Obstruction{{Frame: rect(900, 0, 20, 600), Occluding: true, Separating: true}}, Geometry{
			Regions: []Region{{Frame: rect(0, 0, 800, 600)}},
		}},
		{"a frame past the surface's edge", []Obstruction{{Frame: rect(390, -50, 20, 700), Occluding: true, Separating: true}}, Geometry{
			Regions:      []Region{{Frame: rect(0, 0, 390, 600)}, {Frame: rect(410, 0, 390, 600)}},
			Obstructions: []Obstruction{{Frame: rect(390, 0, 20, 600), Occluding: true, Separating: true}},
		}},
		{"a frame without length", []Obstruction{{Frame: rect(400, 300, 0, 0), Separating: true}}, Geometry{
			Regions: []Region{{Frame: rect(0, 0, 800, 600)}},
		}},
		{"a separating frame that does not cross the surface", []Obstruction{{Frame: rect(390, 100, 20, 400), Occluding: true, Separating: true}}, Geometry{
			Regions:      []Region{{Frame: rect(0, 0, 800, 600), Reserved: rect(390, 100, 20, 400)}},
			Obstructions: []Obstruction{{Frame: rect(390, 100, 20, 400), Occluding: true, Separating: true}},
		}},
	}
	for _, c := range cases {
		if got := Divide(size, c.obstructions); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

func TestDivideOrdersByPosition(t *testing.T) {
	// Two folds crossing: the regions go top to bottom, then left to right, and the obstructions by position whatever order the platform gave.
	vertical := Obstruction{Frame: rect(400, 0, 0, 600), Separating: true}
	horizontal := Obstruction{Frame: rect(0, 300, 800, 0), Horizontal: true, Separating: true}
	g := Divide(Size{Width: 800, Height: 600}, []Obstruction{vertical, horizontal})
	want := []Region{{Frame: rect(0, 0, 400, 300)}, {Frame: rect(400, 0, 400, 300)}, {Frame: rect(0, 300, 400, 300)}, {Frame: rect(400, 300, 400, 300)}}
	if !reflect.DeepEqual(g.Regions, want) {
		t.Errorf("regions %+v, want %+v", g.Regions, want)
	}
	if !reflect.DeepEqual(g.Obstructions, []Obstruction{vertical, horizontal}) {
		t.Errorf("obstructions %+v, want the vertical fold first", g.Obstructions)
	}
}

func TestGeometryStates(t *testing.T) {
	// A fold half closing at the same size changes the geometry; reporting the same state again does not.
	open := Divide(Size{Width: 800, Height: 600}, []Obstruction{{Frame: rect(400, 0, 0, 600), State: HingeOpen}})
	half := Divide(Size{Width: 800, Height: 600}, []Obstruction{{Frame: rect(400, 0, 0, 600), Separating: true, State: HingeHalfOpen}})
	closed := Divide(Size{Width: 800, Height: 600}, []Obstruction{{Frame: rect(400, 0, 0, 600), Separating: true, State: HingeClosed, Angle: 0.1, HasAngle: true}})
	if open.Equal(half) || half.Equal(closed) {
		t.Error("geometries of different states are equal")
	}
	if !half.Equal(half.Clone()) {
		t.Error("a geometry differs from its clone")
	}
	// The device's hinge, apart from any obstruction.
	flat, shut := Divide(Size{Width: 800, Height: 600}, nil), Divide(Size{Width: 800, Height: 600}, nil)
	flat.State, flat.Angle, flat.HasAngle = HingeOpen, 3.14, true
	shut.State, shut.HasAngle = HingeClosed, true
	if flat.Equal(shut) || flat.Equal(Divide(Size{Width: 800, Height: 600}, nil)) {
		t.Error("geometries of different hinge states are equal")
	}
	if c := flat.Clone(); !c.Equal(flat) || c.State != HingeOpen || c.Angle != 3.14 || !c.HasAngle {
		t.Errorf("clone %+v, want the hinge state of %+v", c, flat)
	}
	if !(Geometry{}).Equal(Geometry{Regions: []Region{}, Obstructions: []Obstruction{}}) {
		t.Error("empty lists differ from none")
	}
	if c := half.Clone(); &c.Regions[0] == &half.Regions[0] {
		t.Error("a clone shares its regions")
	}
}

func TestGeometrySerializes(t *testing.T) {
	hinged := Divide(Size{Width: 800, Height: 600}, []Obstruction{{Frame: rect(390, 0, 20, 600), Occluding: true, Separating: true, State: HingeHalfOpen, Angle: 1.5, HasAngle: true}})
	hinged.State, hinged.Angle, hinged.HasAngle = HingeHalfOpen, 1.5, true
	for _, g := range []Geometry{{}, hinged, {Regions: []Region{{Frame: rect(0, 0, 800, 600)}}, State: HingeOpen, Angle: 3.14, HasAngle: true}} {
		data, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		var back Geometry
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		if !back.Equal(g) {
			t.Errorf("%s came back as %+v, want %+v", data, back, g)
		}
	}
}

func TestVerticalBarEdgeStringAndEquality(t *testing.T) {
	cases := []struct {
		edge VerticalBarEdge
		want string
	}{
		{VerticalBarUnspecified, "unspecified"},
		{VerticalBarLeading, "leading"},
		{VerticalBarTrailing, "trailing"},
		{VerticalBarEdge(99), "unspecified"},
	}
	for _, c := range cases {
		if got := c.edge.String(); got != c.want {
			t.Errorf("VerticalBarEdge(%d).String() = %q, want %q", c.edge, got, c.want)
		}
	}
	a := Environment{OS: OSIOS, VerticalBarEdge: VerticalBarLeading}
	b := Environment{OS: OSIOS, VerticalBarEdge: VerticalBarTrailing}
	if a.Equal(b) {
		t.Error("environments with different vertical bar edges compared equal")
	}
	if !a.Equal(Environment{OS: OSIOS, VerticalBarEdge: VerticalBarLeading}) {
		t.Error("identical vertical bar edge environments compared unequal")
	}
}
