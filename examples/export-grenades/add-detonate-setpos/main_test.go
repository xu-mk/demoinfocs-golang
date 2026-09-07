package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchCSVAddsGenGrenadeExplode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "grenades.csv")
	src := utf8BOM + strings.Join([]string{
		"map,demo,grenade_type,detonate_x,detonate_y,detonate_z,setpos/setang,category",
		"de_mirage,m1.dem,smoke,4.000,5.000,6.125,setpos 1.000 2.000 3.000; setang 10.000 90.000,",
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	status, err := patchCSV(path)
	require.NoError(t, err)
	assert.Equal(t, "patched", status)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}))

	got := strings.TrimPrefix(string(data), "\uFEFF")
	assert.Contains(t, got, "gen_grenade_explode")
	assert.Contains(t, got, "gen_grenade_explode smoke 4.000 5.000 6.125")
	assert.True(t, strings.HasSuffix(strings.TrimSpace(strings.Split(got, "\n")[0]), ",category"))

	status, err = patchCSV(path)
	require.NoError(t, err)
	assert.Equal(t, "skipped", status)
}

func TestPatchCSVMapsLegacyChinese(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "old.csv")
	src := "道具所属地图,道具所属demo,道具种类,爆点X,爆点Y,爆点Z,道具分类,setpos/setang\n" +
		"de_mirage,m1.dem,烟,4.000,5.000,6.125,,setpos 1 2 3; setang 10 90\n"
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	status, err := patchCSV(path)
	require.NoError(t, err)
	assert.Equal(t, "patched", status)

	got := string(mustRead(t, path))
	assert.Contains(t, got, "gen_grenade_explode smoke 4.000 5.000 6.125")
	assert.Contains(t, got, "category")
}

func TestPatchCSVIgnoresUnrelated(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "other.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2\n"), 0o600))

	status, err := patchCSV(path)
	require.NoError(t, err)
	assert.Equal(t, "ignored", status)
}

func TestPatchDirRecurses(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "match-1", "extra")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	good := utf8BOM + "grenade_type,detonate_x,detonate_y,detonate_z\nflash,1.0,2.0,3.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.csv"), []byte(good), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "b.csv"), []byte(good), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte("nope"), 0o600))

	var stderr bytes.Buffer
	err := patchDir(root, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "patched 2")

	for _, name := range []string{"a.csv", filepath.Join("match-1", "extra", "b.csv")} {
		data, readErr := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, readErr)
		assert.Contains(t, string(data), "gen_grenade_explode flash 1.0 2.0 3.0")
	}
}

func TestCollectCSVPathsSingleFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "one.csv")
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o600))

	paths, err := collectCSVPaths(path)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.True(t, filepath.IsAbs(paths[0]))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return data
}
