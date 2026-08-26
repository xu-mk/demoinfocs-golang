package main

import (
	"math"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/stretchr/testify/assert"
)

func TestGroundSpeedBetween(t *testing.T) {
	prev := r3.Vector{X: 0, Y: 0, Z: 0}
	cur := r3.Vector{X: 250, Y: 0, Z: 10} // moved 250 units in XY over 1 second
	speed, ok := groundSpeedBetween(prev, cur, 0, 64, 64)
	assert.True(t, ok)
	assert.InDelta(t, 250.0, speed, 0.01)

	_, ok = groundSpeedBetween(prev, cur, 10, 10, 64)
	assert.False(t, ok)

	diag := r3.Vector{X: 3, Y: 4, Z: 0}
	speed, ok = groundSpeedBetween(prev, diag, 0, 64, 64)
	assert.True(t, ok)
	assert.InDelta(t, 5.0, speed, 0.01)
	assert.Equal(t, 5.0, math.Hypot(3, 4))
}
