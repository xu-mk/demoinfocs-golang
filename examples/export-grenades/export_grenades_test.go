package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/geo/r3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
)

func TestGrenadeTypeLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		eq    common.EquipmentType
		label string
		ok    bool
	}{
		{common.EqSmoke, typeSmoke, true},
		{common.EqFlash, typeFlash, true},
		{common.EqHE, typeHE, true},
		{common.EqMolotov, typeFire, true},
		{common.EqIncendiary, typeFire, true},
		{common.EqDecoy, typeDecoy, true},
		{common.EqAK47, "", false},
		{common.EqUnknown, "", false},
	}

	for _, tc := range cases {
		label, ok := grenadeTypeLabel(tc.eq)
		assert.Equal(t, tc.ok, ok, tc.eq.String())
		assert.Equal(t, tc.label, label, tc.eq.String())
	}
}

func TestRecordFromProjectile(t *testing.T) {
	t.Parallel()

	t.Run("nil projectile", func(t *testing.T) {
		t.Parallel()

		_, ok := recordFromProjectile("demo.dem", "de_mirage", nil)
		assert.False(t, ok)
	})

	t.Run("missing weapon", func(t *testing.T) {
		t.Parallel()

		_, ok := recordFromProjectile("demo.dem", "de_mirage", &common.GrenadeProjectile{})
		assert.False(t, ok)
	})

	t.Run("non-grenade weapon", func(t *testing.T) {
		t.Parallel()

		proj := &common.GrenadeProjectile{
			WeaponInstance: &common.Equipment{Type: common.EqAK47},
		}

		_, ok := recordFromProjectile("demo.dem", "de_mirage", proj)
		assert.False(t, ok)
	})

	t.Run("smoke with trajectory and thrower", func(t *testing.T) {
		t.Parallel()

		proj := &common.GrenadeProjectile{
			WeaponInstance: &common.Equipment{Type: common.EqSmoke},
			Thrower:        &common.Player{Name: "s1mple"},
			Trajectory: []common.TrajectoryEntry{
				{Position: r3.Vector{X: 1, Y: 2, Z: 3}},
				{Position: r3.Vector{X: 4, Y: 5, Z: 6}},
				{Position: r3.Vector{X: 7, Y: 8, Z: 9}},
			},
		}

		rec, ok := recordFromProjectile("/tourney/match1/m1.dem", "de_dust2", proj)
		require.True(t, ok)
		assert.Equal(t, "de_dust2", rec.Map)
		assert.Equal(t, "/tourney/match1/m1.dem", rec.DemoPath)
		assert.Equal(t, typeSmoke, rec.GrenadeType)
		assert.Equal(t, "s1mple", rec.Thrower)
		assert.Equal(t, r3.Vector{X: 1, Y: 2, Z: 3}, rec.Start)
		assert.Equal(t, r3.Vector{X: 7, Y: 8, Z: 9}, rec.Detonate)
		assert.Empty(t, rec.Category)
	})

	t.Run("nil thrower", func(t *testing.T) {
		t.Parallel()

		proj := &common.GrenadeProjectile{
			WeaponInstance: &common.Equipment{Type: common.EqFlash},
			Trajectory: []common.TrajectoryEntry{
				{Position: r3.Vector{X: 10, Y: 20, Z: 30}},
			},
		}

		rec, ok := recordFromProjectile("a.dem", "de_nuke", proj)
		require.True(t, ok)
		assert.Equal(t, typeFlash, rec.GrenadeType)
		assert.Empty(t, rec.Thrower)
		assert.Equal(t, r3.Vector{X: 10, Y: 20, Z: 30}, rec.Start)
		assert.Equal(t, r3.Vector{X: 10, Y: 20, Z: 30}, rec.Detonate)
	})
}

func TestWriteCSV(t *testing.T) {
	t.Parallel()

	records := []GrenadeRecord{
		{
			Map:         "de_mirage",
			DemoPath:    "/tourney/liquid-vs-navi/m1-mirage.dem",
			GrenadeType: typeSmoke,
			Thrower:     "s1mple",
			Start:       r3.Vector{X: 1.5, Y: 2.25, Z: 3},
			Detonate:    r3.Vector{X: 4, Y: 5, Z: 6.125},
		},
		{
			Map:         "de_inferno",
			DemoPath:    `/tourney/faze-vs-vitality/m2,inferno.dem`,
			GrenadeType: typeFire,
			Thrower:     "ZywOo, the awper",
			Start:       r3.Vector{X: -100, Y: 0, Z: 64},
			Detonate:    r3.Vector{X: -90.1, Y: 1.2, Z: 70},
		},
	}

	var buf bytes.Buffer

	err := writeCSV(&buf, records)
	require.NoError(t, err)

	raw := buf.Bytes()
	require.True(t, bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}), "CSV must start with UTF-8 BOM for Excel")

	got := strings.TrimPrefix(buf.String(), "\uFEFF")
	assert.True(t, strings.HasPrefix(got, strings.Join(csvHeader, ",")+"\n"))
	assert.Contains(t, got, "道具种类")

	lines := strings.Split(strings.TrimSpace(got), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "de_mirage,/tourney/liquid-vs-navi/m1-mirage.dem,烟,s1mple,1.500,2.250,3.000,4.000,5.000,6.125,", lines[1])
	assert.Contains(t, lines[2], "de_inferno")
	assert.Contains(t, lines[2], "火")
	assert.Contains(t, lines[2], `"ZywOo, the awper"`)
	assert.True(t, strings.HasSuffix(lines[2], ","))
}

func TestCollectDemoPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	match1 := filepath.Join(root, "liquid-vs-navi")
	match2 := filepath.Join(root, "faze-vs-vitality")
	nested := filepath.Join(match2, "extra")

	require.NoError(t, os.MkdirAll(match1, 0o755))
	require.NoError(t, os.MkdirAll(nested, 0o755))

	files := []string{
		filepath.Join(match1, "m1-mirage.dem"),
		filepath.Join(match1, "m2-inferno.DEM"),
		filepath.Join(match1, "readme.txt"),
		filepath.Join(match2, "m1-anubis.dem"),
		filepath.Join(nested, "m2-dust2.dem"),
	}

	for _, f := range files {
		require.NoError(t, os.WriteFile(f, []byte("demo"), 0o600))
	}

	paths, err := collectDemoPaths(root)
	require.NoError(t, err)
	require.Len(t, paths, 4)

	for _, p := range paths {
		assert.True(t, isDemoFile(p))
		assert.True(t, filepath.IsAbs(p))
	}

	assert.True(t, isSorted(paths))
}

func TestCollectDemoPathsSingleFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	demo := filepath.Join(dir, "solo.dem")
	require.NoError(t, os.WriteFile(demo, []byte("demo"), 0o600))

	paths, err := collectDemoPaths(demo)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.True(t, filepath.IsAbs(paths[0]))

	_, err = collectDemoPaths(filepath.Join(dir, "notes.txt"))
	assert.Error(t, err)
}

func TestParseArgs(t *testing.T) {
	t.Parallel()

	_, err := parseArgs(nil)
	assert.Error(t, err)

	opts, err := parseArgs([]string{"-dir", "/tourney", "-out", "out.csv"})
	require.NoError(t, err)
	assert.Equal(t, "/tourney", opts.dir)
	assert.Equal(t, "out.csv", opts.out)

	opts, err = parseArgs([]string{"-demo", "a.dem"})
	require.NoError(t, err)
	assert.Equal(t, "a.dem", opts.demo)
}

func TestExportGrenadesNoDemos(t *testing.T) {
	t.Parallel()

	err := exportGrenades(options{dir: t.TempDir()}, io.Discard, io.Discard)
	assert.Error(t, err)
}

func TestExportTournamentCSV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test")
	}

	demo := filepath.Clean("../../test/cs-demos/s2/s2.dem")
	if _, err := os.Stat(demo); err != nil {
		t.Skip("test demo not available")
	}

	root := t.TempDir()
	matchDir := filepath.Join(root, "match-1")
	require.NoError(t, os.MkdirAll(matchDir, 0o755))

	linked := filepath.Join(matchDir, "s2.dem")
	absDemo, err := filepath.Abs(demo)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(absDemo, linked))

	outFile := filepath.Join(t.TempDir(), "grenades.csv")

	var stderr bytes.Buffer

	err = exportGrenades(options{dir: root, out: outFile}, io.Discard, &stderr)
	require.NoError(t, err)

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)

	require.True(t, bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}), "CSV must start with UTF-8 BOM for Excel")

	text := strings.TrimPrefix(string(data), "\uFEFF")
	assert.True(t, strings.HasPrefix(text, strings.Join(csvHeader, ",")+"\n"))
	assert.Contains(t, text, "道具种类")

	lines := strings.Split(strings.TrimSpace(text), "\n")
	require.Greater(t, len(lines), 1, "expected at least one grenade row")

	for _, line := range lines[1:] {
		assert.True(t, strings.HasSuffix(line, ",") || strings.Contains(line, ",,"), "category column should be empty: %s", line)
	}

	assert.Contains(t, stderr.String(), "exported")
}

func TestIsDemoFile(t *testing.T) {
	t.Parallel()

	assert.True(t, isDemoFile("a.dem"))
	assert.True(t, isDemoFile("A.DEM"))
	assert.True(t, isDemoFile("/x/y.Dem"))
	assert.False(t, isDemoFile("a.txt"))
	assert.False(t, isDemoFile("demo"))
}

func isSorted(vals []string) bool {
	for i := 1; i < len(vals); i++ {
		if vals[i-1] > vals[i] {
			return false
		}
	}

	return true
}
