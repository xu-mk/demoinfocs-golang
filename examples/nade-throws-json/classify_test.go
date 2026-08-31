package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
)

func TestFormatThrowMethodExamples(t *testing.T) {
	tests := []struct {
		name string
		in   throwClassificationInput
		want string
	}{
		{
			name: "蹲w跑一步跳左键投",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Forward:       true,
				Duck:          true,
				Airborne:      true,
				GroundSpeed:   130,
			},
			want: "蹲w跑一步跳左键投",
		},
		{
			name: "w跳左键投 (air strafe ~30)",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Forward:       true,
				Airborne:      true,
				GroundSpeed:   30,
			},
			want: "w跳左键投",
		},
		{
			name: "d跑跳左键投",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Right:         true,
				Airborne:      true,
				GroundSpeed:   245,
			},
			want: "d跑跳左键投",
		},
		{
			name: "右键站投",
			in: throwClassificationInput{
				RightClickHeld: true,
				GroundSpeed:    0,
			},
			want: "右键站投",
		},
		{
			name: "地速为0时忽略残留方向键",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Forward:       true,
				GroundSpeed:   0,
			},
			want: "左键站投",
		},
		{
			name: "蹲投需投掷瞬间按住蹲，地速为0不加方向",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Duck:          true,
				Forward:       true, // leftover; ignored because speed is 0
				GroundSpeed:   0,
			},
			want: "蹲左键投",
		},
		{
			name: "w跑一步左键投",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Forward:       true,
				GroundSpeed:   130,
			},
			want: "w跑一步左键投",
		},
		{
			name: "双键站跳投",
			in: throwClassificationInput{
				LeftClickHeld:  true,
				RightClickHeld: true,
				Airborne:       true,
			},
			want: "双键站跳投",
		},
		{
			name: "shiftw投",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Forward:       true,
				Walk:          true,
				GroundSpeed:   130,
			},
			want: "shiftw左键投",
		},
		{
			name: "wd跑左键投",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Forward:       true,
				Right:         true,
				GroundSpeed:   245,
			},
			want: "wd跑左键投",
		},
		{
			name: "地速为0时忽略静走，只标蹲",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Walk:          true,
				Duck:          true,
				GroundSpeed:   0,
			},
			want: "蹲左键投",
		},
		{
			name: "地速为0且静走未蹲则为站投",
			in: throwClassificationInput{
				LeftClickHeld: true,
				Walk:          true,
				GroundSpeed:   0,
			},
			want: "左键站投",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatThrowMethod(tc.in))
		})
	}
}

func TestLastGrenadeClickHold(t *testing.T) {
	a := uint64(common.ButtonAttack)
	a2 := uint64(common.ButtonAttack2)
	both := a | a2

	left := []timedButtons{
		{tick: 10, state: a},
		{tick: 20, state: 0},
	}
	atk, atk2 := lastGrenadeClickHold(left, 20)
	assert.True(t, atk)
	assert.False(t, atk2)
	assert.Equal(t, "左键", classifyMouse(throwClassificationInput{LeftClickHeld: atk, RightClickHeld: atk2}))

	right := []timedButtons{
		{tick: 10, state: a2},
		{tick: 20, state: 0},
	}
	atk, atk2 = lastGrenadeClickHold(right, 20)
	assert.False(t, atk)
	assert.True(t, atk2)

	dual := []timedButtons{
		{tick: 10, state: a},
		{tick: 12, state: both},
		{tick: 20, state: 0},
	}
	atk, atk2 = lastGrenadeClickHold(dual, 20)
	assert.True(t, atk)
	assert.True(t, atk2)

	// Earlier rifle spray should not override the last nade cook.
	mixed := []timedButtons{
		{tick: 1, state: a},
		{tick: 2, state: 0},
		{tick: 50, state: a2},
		{tick: 60, state: 0},
	}
	atk, atk2 = lastGrenadeClickHold(mixed, 60)
	assert.False(t, atk)
	assert.True(t, atk2)
}

func TestLastDirectionHold(t *testing.T) {
	w := uint64(common.ButtonForward)
	d := uint64(common.ButtonMoveRight)

	// 跑一步: tap W, release, then throw.
	released := []timedButtons{
		{tick: 10, state: w},
		{tick: 20, state: 0},
	}
	f, b, l, r := lastDirectionHold(released, 30)
	assert.True(t, f)
	assert.False(t, b)
	assert.False(t, l)
	assert.False(t, r)

	// 跑投: still holding D at throw.
	held := []timedButtons{
		{tick: 10, state: d},
		{tick: 30, state: d},
	}
	f, b, l, r = lastDirectionHold(held, 30)
	assert.True(t, r)
	assert.False(t, f)

	// Older A tap should not override the last D hold.
	a := uint64(common.ButtonMoveLeft)
	mixed := []timedButtons{
		{tick: 1, state: a},
		{tick: 2, state: 0},
		{tick: 40, state: d},
		{tick: 50, state: 0},
	}
	f, b, l, r = lastDirectionHold(mixed, 60)
	assert.True(t, r)
	assert.False(t, l)
}
