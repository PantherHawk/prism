// Package motion animates presentation values.
//
// Only presentation values: scales, offsets, opacities. Data is never
// interpolated. A chart that eases between two readings is drawing numbers
// nobody observed, and an operator cannot tell that from a real transient.
// Everything in this package moves the frame around the data, never the data.
package motion

import (
	"math"

	"github.com/charmbracelet/harmonica"
)

// Rate is the frame rate the springs are integrated at. It has to match the
// interval the caller ticks them at or the physics runs fast or slow.
const Rate = 60

// settled is the distance from target, relative to the last displacement, at
// which a spring is considered to have arrived. Without it a spring converges
// asymptotically and the render loop never stops.
const settled = 1e-3

// Spring is a damped harmonic oscillator driving one scalar.
type Spring struct {
	spring   harmonica.Spring
	position float64
	velocity float64
	target   float64
	scale    float64
	moving   bool
}

// NewSpring returns a spring.
//
// frequency is how fast it converges, in radians per second; damping is the
// ratio, where 1 is critical. Below 1 overshoots, which reads as physical for
// a control the operator is driving, and reads as a lie for anything showing
// data.
func NewSpring(frequency, damping float64) *Spring {
	return &Spring{
		spring: harmonica.NewSpring(harmonica.FPS(Rate), frequency, damping),
		scale:  1,
	}
}

// Snap moves the spring to a value immediately, with no motion.
//
// Used for the first frame and for terminal resizes: animating from a stale
// layout to a new one after a resize looks like a bug, not a flourish.
func (s *Spring) Snap(value float64) {
	s.position, s.target, s.velocity, s.moving = value, value, 0, false
}

// SetTarget aims the spring at a value.
func (s *Spring) SetTarget(value float64) {
	if value == s.target {
		return
	}

	s.scale = math.Max(math.Abs(value-s.position), math.Abs(value))
	s.target = value
	s.moving = true
}

// Target returns the value the spring is heading for, which is the value the
// operator asked for. Decisions read this; only rendering reads [Spring.Value].
func (s *Spring) Target() float64 {
	return s.target
}

// Value returns the current animated position.
func (s *Spring) Value() float64 {
	return s.position
}

// Moving reports whether the spring still has somewhere to go.
func (s *Spring) Moving() bool {
	return s.moving
}

// Update advances the spring by one frame and reports whether it is still
// moving.
func (s *Spring) Update() bool {
	if !s.moving {
		return false
	}

	s.position, s.velocity = s.spring.Update(s.position, s.velocity, s.target)

	tolerance := settled * math.Max(s.scale, 1)
	if math.Abs(s.target-s.position) < tolerance && math.Abs(s.velocity) < tolerance {
		s.position, s.velocity, s.moving = s.target, 0, false
	}

	return s.moving
}

// Group advances several springs together.
//
// One ticker drives every animation in the program, and it stops the moment
// nothing is moving. An idle TUI should cost nothing: a dashboard left open on
// a second monitor must not burn a core redrawing an unchanged frame.
type Group struct {
	springs []*Spring
}

// NewGroup returns a group over the given springs.
func NewGroup(springs ...*Spring) *Group {
	return &Group{springs: springs}
}

// Update advances every spring and reports whether any is still moving.
func (g *Group) Update() bool {
	moving := false

	for _, spring := range g.springs {
		if spring.Update() {
			moving = true
		}
	}

	return moving
}

// Moving reports whether any spring is in motion.
func (g *Group) Moving() bool {
	for _, spring := range g.springs {
		if spring.Moving() {
			return true
		}
	}

	return false
}
