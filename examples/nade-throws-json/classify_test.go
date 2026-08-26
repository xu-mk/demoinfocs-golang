package main

import (
	"testing"

	"github.com/golang/geo/r3"
	"github.com/stretchr/testify/assert"
)

func TestClassifyThrowMethod(t *testing.T) {
	stand := []r3.Vector{
		{X: 100, Y: 200, Z: 10},
		{X: 100.5, Y: 200.2, Z: 10.1},
		{X: 100.3, Y: 199.8, Z: 10.0},
	}
	assert.Equal(t, ThrowMethodStandingStill, classifyThrowMethod(stand, false))

	standJump := []r3.Vector{
		{X: 100, Y: 200, Z: 10},
		{X: 100.2, Y: 200.1, Z: 30},
		{X: 100.1, Y: 199.9, Z: 50},
	}
	assert.Equal(t, ThrowMethodStandingJump, classifyThrowMethod(standJump, true))

	runJump := []r3.Vector{
		{X: 100, Y: 200, Z: 10},
		{X: 120, Y: 210, Z: 30},
		{X: 140, Y: 220, Z: 50},
	}
	assert.Equal(t, ThrowMethodRunningJump, classifyThrowMethod(runJump, true))

	// Airborne with no positional change still matches standing_still by the
	// movement rule (xyz unchanged). Jump classification requires Z movement.
	assert.Equal(t, ThrowMethodStandingStill, classifyThrowMethod(stand, true))

	// Grounded movement is outside the three requested classes.
	runOnly := []r3.Vector{
		{X: 100, Y: 200, Z: 10},
		{X: 130, Y: 200, Z: 10},
	}
	assert.Equal(t, ThrowMethodOther, classifyThrowMethod(runOnly, false))
}
