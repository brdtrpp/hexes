package game

import "math"

// Stick is a floating on-screen joystick: it anchors where the thumb
// lands and reads the drag from there as an analog direction and speed.
// Dragging past the rim pulls the anchor along, so the stick never runs
// out of travel. It knows nothing about devices; the front end feeds it
// screen points.
type Stick struct {
	On       bool
	ID       int     // which touch (or the mouse) is holding it
	BaseX    float64 // anchor, screen pixels
	BaseY    float64
	X, Y     float64 // where the thumb is
	Radius   float64 // full deflection, screen pixels
	Deadzone float64 // share of Radius that reads as zero
}

// Begin anchors the stick under a new touch.
func (s *Stick) Begin(id int, x, y float64) {
	s.On, s.ID = true, id
	s.BaseX, s.BaseY, s.X, s.Y = x, y, x, y
}

// Move follows the thumb.
func (s *Stick) Move(x, y float64) {
	s.X, s.Y = x, y
	dx, dy := x-s.BaseX, y-s.BaseY
	if d := math.Hypot(dx, dy); d > s.Radius && s.Radius > 0 {
		k := (d - s.Radius) / d
		s.BaseX += dx * k
		s.BaseY += dy * k
	}
}

// End lets go.
func (s *Stick) End() { s.On = false }

// Knob is where to draw the knob: the thumb, kept inside the rim.
func (s *Stick) Knob() (float64, float64) {
	dx, dy := s.X-s.BaseX, s.Y-s.BaseY
	if d := math.Hypot(dx, dy); d > s.Radius && d > 0 {
		dx, dy = dx/d*s.Radius, dy/d*s.Radius
	}
	return s.BaseX + dx, s.BaseY + dy
}

// Vector is the movement it asks for: length 0 inside the deadzone,
// rising to 1 at the rim, so a light push walks slowly.
func (s *Stick) Vector() (float64, float64) {
	if !s.On || s.Radius <= 0 {
		return 0, 0
	}
	dx, dy := s.X-s.BaseX, s.Y-s.BaseY
	d := math.Hypot(dx, dy)
	m := min(1, d/s.Radius)
	if m <= s.Deadzone || d == 0 {
		return 0, 0
	}
	m = (m - s.Deadzone) / (1 - s.Deadzone)
	return dx / d * m, dy / d * m
}
