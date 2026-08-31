package main

import (
	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
)

const (
	runSpeedMin       = 190 // ~245 跑投
	runStepSpeedMin   = 80  // ~130 跑一步投
	airStrafeSpeedMax = 60  // ~30 跳起后按方向键
)

type timedButtons struct {
	tick  int
	state uint64
}

type throwClassificationInput struct {
	LeftClickHeld  bool
	RightClickHeld bool
	Forward        bool
	Back           bool
	Left           bool
	Right          bool
	Duck           bool
	Walk           bool
	Airborne       bool
	GroundSpeed    float64
}

func buttonDown(state uint64, mask demoinfocs.ButtonBitMask) bool {
	return state&uint64(mask) != 0
}

func lastGrenadeClickHold(samples []timedButtons, throwTick int) (attack, attack2 bool) {
	idx := -1
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].tick > throwTick {
			continue
		}
		if buttonDown(samples[i].state, demoinfocs.ButtonAttack) || buttonDown(samples[i].state, demoinfocs.ButtonAttack2) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false, false
	}

	start := idx
	for start > 0 {
		prev := samples[start-1]
		if !buttonDown(prev.state, demoinfocs.ButtonAttack) && !buttonDown(prev.state, demoinfocs.ButtonAttack2) {
			break
		}
		start--
	}

	for i := start; i < len(samples); i++ {
		s := samples[i]
		if s.tick > throwTick {
			break
		}
		if i > start && !buttonDown(s.state, demoinfocs.ButtonAttack) && !buttonDown(s.state, demoinfocs.ButtonAttack2) {
			break
		}
		if buttonDown(s.state, demoinfocs.ButtonAttack) {
			attack = true
		}
		if buttonDown(s.state, demoinfocs.ButtonAttack2) {
			attack2 = true
		}
	}
	return attack, attack2
}

func classifyMouse(in throwClassificationInput) string {
	switch {
	case in.LeftClickHeld && in.RightClickHeld:
		return "双键"
	case in.LeftClickHeld:
		return "左键"
	case in.RightClickHeld:
		return "右键"
	default:
		return ""
	}
}

func movementFromMask(state uint64) (forward, back, left, right, duck, walk bool) {
	return buttonDown(state, demoinfocs.ButtonForward),
		buttonDown(state, demoinfocs.ButtonBack),
		buttonDown(state, demoinfocs.ButtonMoveLeft),
		buttonDown(state, demoinfocs.ButtonMoveRight),
		buttonDown(state, demoinfocs.ButtonDuck),
		buttonDown(state, demoinfocs.ButtonSpeed)
}

func classifySpecial(in throwClassificationInput) string {
	s := ""
	if in.Duck {
		s += "蹲"
	}
	if in.Walk {
		s += "shift"
	}
	return s
}

func classifyDirections(in throwClassificationInput) string {
	s := ""
	if in.Forward {
		s += "w"
	}
	if in.Left {
		s += "a"
	}
	if in.Back {
		s += "s"
	}
	if in.Right {
		s += "d"
	}
	return s
}

func hasDirection(in throwClassificationInput) bool {
	return in.Forward || in.Back || in.Left || in.Right
}

func classifyMoveDegree(in throwClassificationInput) string {
	// Walk (shift) already implies a reduced speed band.
	if in.Walk {
		return ""
	}
	// Jump + direction at ~30 u/s is air-strafe, e.g. "w跳投".
	if in.Airborne && hasDirection(in) && in.GroundSpeed <= airStrafeSpeedMax {
		return ""
	}
	if in.GroundSpeed >= runSpeedMin {
		return "跑"
	}
	if in.GroundSpeed >= runStepSpeedMin {
		return "跑一步"
	}
	return ""
}

// formatThrowMethod builds labels such as "蹲w跳左键投", "d跑跳左键投", "右键站投".
func formatThrowMethod(in throwClassificationInput) string {
	mouse := classifyMouse(in)
	special := classifySpecial(in)
	dirs := classifyDirections(in)
	speed := classifyMoveDegree(in)
	jump := ""
	if in.Airborne {
		jump = "跳"
	}

	if special == "" && dirs == "" {
		return mouse + "站" + jump + "投"
	}
	return special + dirs + speed + jump + mouse + "投"
}
