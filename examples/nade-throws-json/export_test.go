package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExportDemoThrowsFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping demo parse in short mode")
	}

	demoPath := "../../test/cs-demos/s2/s2.dem"
	if _, err := os.Stat(demoPath); err != nil {
		demoPath = "../../test/cs-demos/s2/s2/s2.dem"
		if _, err := os.Stat(demoPath); err != nil {
			t.Skip("test demo not available")
		}
	}

	data, err := ExportDemoThrowsFile(demoPath)
	assert.NoError(t, err)
	assert.NotEmpty(t, data.Map)
	assert.NotEmpty(t, data.Rounds)

	total := 0
	for _, round := range data.Rounds {
		total += len(round.Grenades)
		for _, g := range round.Grenades {
			assert.NotEmpty(t, g.Type)
			assert.NotZero(t, g.ID)
			assert.Contains(t, []string{
				ThrowMethodStandingStill,
				ThrowMethodStandingJump,
				ThrowMethodRunningJump,
				ThrowMethodOther,
			}, g.ThrowMethod)
		}
	}
	assert.Greater(t, total, 0)
}
