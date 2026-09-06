package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	utf8BOM     = "\uFEFF"
	colDetonate = "setpos爆点"
	colX        = "爆点X"
	colY        = "爆点Y"
	colZ        = "爆点Z"
)

func main() {
	dir := flag.String("dir", "", "Folder to scan for exported grenade CSVs (recursive)")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(1)
	}

	if err := patchDir(*dir, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func patchDir(root string, stderr io.Writer) error {
	paths, err := collectCSVPaths(root)
	if err != nil {
		return err
	}

	if len(paths) == 0 {
		return fmt.Errorf("no .csv files found under %s", root)
	}

	var patched, skipped, ignored, failed int

	for _, path := range paths {
		rel := path
		if r, relErr := filepath.Rel(root, path); relErr == nil {
			rel = filepath.ToSlash(r)
		}

		status, patchErr := patchCSV(path)
		if patchErr != nil {
			failed++
			fmt.Fprintf(stderr, "error %s: %v\n", rel, patchErr)

			continue
		}

		switch status {
		case "patched":
			patched++
			fmt.Fprintf(stderr, "patched %s\n", rel)
		case "skipped":
			skipped++
			fmt.Fprintf(stderr, "skip %s (already has %s)\n", rel, colDetonate)
		default:
			ignored++
			fmt.Fprintf(stderr, "ignore %s (not a grenade export CSV)\n", rel)
		}
	}

	fmt.Fprintf(stderr, "done: patched %d, skipped %d, ignored %d, failed %d\n", patched, skipped, ignored, failed)

	if failed > 0 {
		return fmt.Errorf("failed to patch %d file(s)", failed)
	}

	return nil
}

func collectCSVPaths(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s: %w", root, err)
	}

	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(root), ".csv") {
			abs, absErr := filepath.Abs(root)
			if absErr != nil {
				return nil, absErr
			}

			return []string{abs}, nil
		}

		return nil, fmt.Errorf("not a directory or .csv file: %s", root)
	}

	var paths []string

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".csv") {
			return nil
		}

		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return absErr
		}

		paths = append(paths, abs)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk %s: %w", root, err)
	}

	return paths, nil
}

func patchCSV(path string) (string, error) {
	rows, err := readCSV(path)
	if err != nil {
		return "", err
	}

	if len(rows) == 0 {
		return "ignored", nil
	}

	header := rows[0]
	if indexOf(header, colDetonate) >= 0 {
		return "skipped", nil
	}

	xCol := indexOf(header, colX)
	yCol := indexOf(header, colY)
	zCol := indexOf(header, colZ)
	if xCol < 0 || yCol < 0 || zCol < 0 {
		return "ignored", nil
	}

	rows[0] = append(header, colDetonate)

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		x, y, z := cell(row, xCol), cell(row, yCol), cell(row, zCol)
		rows[i] = append(row, "setpos "+x+" "+y+" "+z)
	}

	err = writeCSV(path, rows)
	if err != nil {
		return "", err
	}

	return "patched", nil
}

func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(stripBOM(f))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	return rows, nil
}

func writeCSV(path string, rows [][]string) error {
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	_, err = io.WriteString(f, utf8BOM)
	if err != nil {
		f.Close()
		_ = os.Remove(tmp)

		return err
	}

	w := csv.NewWriter(f)
	err = w.WriteAll(rows)
	if err != nil {
		f.Close()
		_ = os.Remove(tmp)

		return fmt.Errorf("failed to write CSV: %w", err)
	}

	err = f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}

	err = os.Rename(tmp, path)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}

	return nil
}

func stripBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	if peek, err := br.Peek(3); err == nil && len(peek) == 3 && peek[0] == 0xEF && peek[1] == 0xBB && peek[2] == 0xBF {
		_, _ = br.Discard(3)
	}

	return br
}

func indexOf(vals []string, want string) int {
	for i, v := range vals {
		if v == want {
			return i
		}
	}

	return -1
}

func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}

	return row[i]
}
