package game

import (
	"math"
	"testing"
)

func TestStick(t *testing.T) {
	s := Stick{Radius: 100, Deadzone: 0.15}
	if x, y := s.Vector(); x != 0 || y != 0 {
		t.Fatal("idle stick moves")
	}
	s.Begin(3, 200, 300)
	s.Move(210, 300) // inside the deadzone
	if x, y := s.Vector(); x != 0 || y != 0 {
		t.Fatalf("deadzone leaked (%.2f, %.2f)", x, y)
	}
	s.Move(200, 300-57.5) // halfway between deadzone and rim, straight up
	if x, y := s.Vector(); math.Abs(x) > 1e-9 || math.Abs(y+0.5) > 1e-9 {
		t.Fatalf("half push read (%.3f, %.3f), want (0, -0.5)", x, y)
	}
	s.Move(500, 300) // way past the rim: full speed, anchor follows
	if x, y := s.Vector(); math.Abs(x-1) > 1e-9 || math.Abs(y) > 1e-9 {
		t.Fatalf("full push read (%.3f, %.3f)", x, y)
	}
	if s.BaseX != 400 {
		t.Fatalf("anchor at %.1f, want dragged to 400", s.BaseX)
	}
	s.Move(400, 300) // back to the anchor: stop, no snap
	if x, y := s.Vector(); x != 0 || y != 0 {
		t.Fatalf("centered stick reads (%.2f, %.2f)", x, y)
	}
	s.End()
	if x, y := s.Vector(); x != 0 || y != 0 || s.On {
		t.Fatal("released stick still moves")
	}
}

func TestStickDrivesAnalogSpeed(t *testing.T) {
	s := openArena(30, 30)
	p := s.P()
	st := Stick{Radius: 100, Deadzone: 0.15}
	st.Begin(0, 0, 0)
	st.Move(57.5, 0) // half speed east
	x0 := p.X
	mx, my := st.Vector()
	run(s, mx, my, 0.5, nil)
	want := PlayerSpeed * tileSpan * 0.5 * 0.5
	if got := p.X - x0; math.Abs(got-want) > want*0.01 {
		t.Fatalf("half stick moved %.3f, want %.3f", got, want)
	}
}
