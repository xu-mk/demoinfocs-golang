package main

import (
	"math"

	"github.com/golang/geo/r3"
)

const (
	ThrowMethodStandingStill = "standing_still" // 站立不动投掷
	ThrowMethodStandingJump  = "standing_jump"  // 站立不动跳投
	ThrowMethodRunningJump   = "running_jump"   // 跑跳投
	ThrowMethodOther         = "other"
)

// Movement thresholds in Source units. Demo positions jitter slightly even when
// the player intends to stand still, so exact equality is not usable.
// Intentional running typically moves far more than these thresholds within the
// lookback window; standing jump still keeps XY near zero.
const (
	defaultXYMoveEpsilon = 10.0
	defaultZMoveEpsilon  = 4.0
)

type throwMovement struct {
	XYMoved   bool
	ZMoved    bool
	Airborne  bool
	XYDelta   float64
	ZDelta    float64
}

func analyzeThrowMovement(positions []r3.Vector, airborne bool, xyEps, zEps float64) throwMovement {
	m := throwMovement{Airborne: airborne}
	if len(positions) < 2 {
		return m
	}

	ref := positions[len(positions)-1]
	for _, pos := range positions[:len(positions)-1] {
		xy := math.Hypot(pos.X-ref.X, pos.Y-ref.Y)
		z := math.Abs(pos.Z - ref.Z)
		if xy > m.XYDelta {
			m.XYDelta = xy
		}
		if z > m.ZDelta {
			m.ZDelta = z
		}
	}

	m.XYMoved = m.XYDelta > xyEps
	m.ZMoved = m.ZDelta > zEps
	return m
}

func classifyThrowMethod(positions []r3.Vector, airborne bool) string {
	return classifyThrowMethodWithEpsilon(positions, airborne, defaultXYMoveEpsilon, defaultZMoveEpsilon)
}

func classifyThrowMethodWithEpsilon(positions []r3.Vector, airborne bool, xyEps, zEps float64) string {
	m := analyzeThrowMovement(positions, airborne, xyEps, zEps)

	switch {
	case !m.XYMoved && !m.ZMoved:
		return ThrowMethodStandingStill
	case m.Airborne && !m.XYMoved && m.ZMoved:
		return ThrowMethodStandingJump
	case m.Airborne && m.XYMoved && m.ZMoved:
		return ThrowMethodRunningJump
	default:
		return ThrowMethodOther
	}
}
