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
		{common.EqMolotov, typeMolotov, true},
		{common.EqIncendiary, typeIncendiary, true},
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

		_, ok := recordFromProjectile("demo.dem", "de_mirage", nil, throwSnapshot{})
		assert.False(t, ok)
	})

	t.Run("missing weapon", func(t *testing.T) {
		t.Parallel()

		_, ok := recordFromProjectile("demo.dem", "de_mirage", &common.GrenadeProjectile{}, throwSnapshot{})
		assert.False(t, ok)
	})

	t.Run("non-grenade weapon", func(t *testing.T) {
		t.Parallel()

		proj := &common.GrenadeProjectile{
			WeaponInstance: &common.Equipment{Type: common.EqAK47},
		}

		_, ok := recordFromProjectile("demo.dem", "de_mirage", proj, throwSnapshot{})
		assert.False(t, ok)
	})

	t.Run("smoke uses thrower pose not projectile start", func(t *testing.T) {
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
		snap := throwSnapshot{
			ok:     true,
			pos:    r3.Vector{X: 100, Y: 200, Z: 64},
			angles: r3.Vector{X: 12.5, Y: 180, Z: 0},
		}

		rec, ok := recordFromProjectile("/tourney/match1/m1.dem", "de_dust2", proj, snap)
		require.True(t, ok)
		assert.Equal(t, "de_dust2", rec.Map)
		assert.Equal(t, "/tourney/match1/m1.dem", rec.DemoPath)
		assert.Equal(t, typeSmoke, rec.GrenadeType)
		assert.Equal(t, "s1mple", rec.Thrower)
		assert.Equal(t, snap.pos, rec.Start)
		assert.Equal(t, snap.angles, rec.ViewAngles)
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

		rec, ok := recordFromProjectile("a.dem", "de_nuke", proj, throwSnapshot{})
		require.True(t, ok)
		assert.Equal(t, typeFlash, rec.GrenadeType)
		assert.Empty(t, rec.Thrower)
		assert.Equal(t, r3.Vector{}, rec.Start)
		assert.Equal(t, r3.Vector{X: 10, Y: 20, Z: 30}, rec.Detonate)
	})
}

func TestWriteCSV(t *testing.T) {
	t.Parallel()

	records := []GrenadeRecord{
		{
			Map:         "de_mirage",
			DemoPath:    "liquid-vs-navi/m1-mirage.dem",
			GrenadeType: typeSmoke,
			Thrower:     "s1mple",
			Start:       r3.Vector{X: 1.5, Y: 2.25, Z: 3},
			ViewAngles:  r3.Vector{X: 10, Y: 90, Z: 0},
			Detonate:    r3.Vector{X: 4, Y: 5, Z: 6.125},
		},
		{
			Map:         "de_inferno",
			DemoPath:    `/tourney/faze-vs-vitality/m2,inferno.dem`,
			GrenadeType: typeMolotov,
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
	assert.Contains(t, got, "grenade_type")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(strings.Split(got, "\n")[0]), ",category"))

	lines := strings.Split(strings.TrimSpace(got), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "de_mirage,liquid-vs-navi/m1-mirage.dem,smoke,s1mple,1.500,2.250,3.000,10.000,90.000,4.000,5.000,6.125,setpos 1.500 2.250 3.000; setang 10.000 90.000,gen_grenade_explode smoke 4.000 5.000 6.125,", lines[1])
	assert.Contains(t, got, "view_x")
	assert.NotContains(t, got, "准星角度")
	assert.Contains(t, got, "setpos/setang")
	assert.Contains(t, got, "gen_grenade_explode")
	assert.Contains(t, lines[2], "de_inferno")
	assert.Contains(t, lines[2], "molotov")
	assert.Contains(t, lines[2], `"ZywOo, the awper"`)
	assert.Contains(t, lines[2], "setpos -100.000 0.000 64.000; setang 0.000 0.000")
	assert.Contains(t, lines[2], "gen_grenade_explode molotov -90.100 1.200 70.000")
	assert.True(t, strings.HasSuffix(lines[1], ","))
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

	refs, err := collectDemoPaths(root)
	require.NoError(t, err)
	require.Len(t, refs, 4)

	rels := make([]string, 0, len(refs))
	for _, ref := range refs {
		assert.True(t, isDemoFile(ref.abs))
		assert.True(t, filepath.IsAbs(ref.abs))
		assert.False(t, filepath.IsAbs(ref.rel))
		rels = append(rels, ref.rel)
	}

	assert.Contains(t, rels, "liquid-vs-navi/m1-mirage.dem")
	assert.Contains(t, rels, "faze-vs-vitality/m1-anubis.dem")
	assert.Contains(t, rels, "faze-vs-vitality/extra/m2-dust2.dem")
	assert.True(t, isSorted(rels))
}

func TestCollectDemoPathsSingleFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	demo := filepath.Join(dir, "solo.dem")
	require.NoError(t, os.WriteFile(demo, []byte("demo"), 0o600))

	refs, err := collectDemoPaths(demo)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.True(t, filepath.IsAbs(refs[0].abs))
	assert.Equal(t, "solo.dem", refs[0].rel)

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
	assert.False(t, opts.overwrite)

	opts, err = parseArgs([]string{"-dir", "/tourney", "-out", "out.csv", "-overwrite"})
	require.NoError(t, err)
	assert.True(t, opts.overwrite)
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
	assert.Contains(t, text, "grenade_type")
	assert.Contains(t, text, "view_x")
	assert.NotContains(t, text, "准星角度")
	assert.Contains(t, text, "setpos/setang")
	assert.Contains(t, text, "gen_grenade_explode")
	assert.Contains(t, text, ",category")
	assert.Contains(t, text, "match-1/s2.dem")
	assert.NotContains(t, text, absDemo)
	assert.Contains(t, text, "setpos ")
	assert.Contains(t, text, "setang ")

	htmlPath := strings.TrimSuffix(outFile, filepath.Ext(outFile)) + ".html"
	_, htmlErr := os.Stat(htmlPath)
	assert.True(t, os.IsNotExist(htmlErr))

	lines := strings.Split(strings.TrimSpace(text), "\n")
	require.Greater(t, len(lines), 1, "expected at least one grenade row")

	assert.Contains(t, stderr.String(), "exported")

	firstLines := len(lines)

	var stderr2 bytes.Buffer
	err = exportGrenades(options{dir: root, out: outFile}, io.Discard, &stderr2)
	require.NoError(t, err)
	assert.Contains(t, stderr2.String(), "already exported")

	data2, err := os.ReadFile(outFile)
	require.NoError(t, err)
	lines2 := strings.Split(strings.TrimSpace(strings.TrimPrefix(string(data2), "\uFEFF")), "\n")
	assert.Equal(t, firstLines, len(lines2), "resume should not duplicate rows")
}

func TestLoadCompletedDemos(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "out.csv")
	progress := csvPath + ".progress"

	var buf bytes.Buffer
	require.NoError(t, writeCSV(&buf, []GrenadeRecord{
		{DemoPath: "match-1/a.dem", GrenadeType: typeSmoke},
		{DemoPath: "match-1/a.dem", GrenadeType: typeFlash},
		{DemoPath: "match-2/b.dem", GrenadeType: typeHE},
	}))
	require.NoError(t, os.WriteFile(csvPath, buf.Bytes(), 0o600))
	require.NoError(t, os.WriteFile(progress, []byte("match-3/empty.dem\n"), 0o600))

	done, err := loadCompletedDemos(csvPath, progress)
	require.NoError(t, err)
	assert.Contains(t, done, "match-1/a.dem")
	assert.Contains(t, done, "match-2/b.dem")
	assert.Contains(t, done, "match-3/empty.dem")
}

func TestExportContinuesOnCorruptDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test")
	}

	demo := filepath.Clean("../../test/cs-demos/s2/s2.dem")
	if _, err := os.Stat(demo); err != nil {
		t.Skip("test demo not available")
	}

	root := t.TempDir()
	badDir := filepath.Join(root, "match-bad")
	goodDir := filepath.Join(root, "match-good")
	require.NoError(t, os.MkdirAll(badDir, 0o755))
	require.NoError(t, os.MkdirAll(goodDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(badDir, "broken.dem"), []byte("not a demo"), 0o600))

	absDemo, err := filepath.Abs(demo)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(absDemo, filepath.Join(goodDir, "s2.dem")))

	outFile := filepath.Join(t.TempDir(), "grenades.csv")
	var stderr bytes.Buffer
	err = exportGrenades(options{dir: root, out: outFile}, io.Discard, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "error")
	assert.Contains(t, stderr.String(), "failed 1")

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "match-good/s2.dem")
}

func TestGenGrenadeExplode(t *testing.T) {
	t.Parallel()

	got := genGrenadeExplode(GrenadeRecord{
		GrenadeType: typeSmoke,
		Detonate:    r3.Vector{X: 4, Y: 5, Z: 6.125},
	})
	assert.Equal(t, "gen_grenade_explode smoke 4.000 5.000 6.125", got)
}

func TestCopyPayload(t *testing.T) {
	t.Parallel()

	got := copyPayload(GrenadeRecord{
		Start:      r3.Vector{X: 1.5, Y: 2.25, Z: 3},
		ViewAngles: r3.Vector{X: 10, Y: 90, Z: 0},
	})
	assert.Equal(t, "setpos 1.500 2.250 3.000; setang 10.000 90.000", got)
}

func TestCaptureThrowSnapshotNil(t *testing.T) {
	t.Parallel()

	assert.Equal(t, throwSnapshot{}, captureThrowSnapshot(nil))
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
