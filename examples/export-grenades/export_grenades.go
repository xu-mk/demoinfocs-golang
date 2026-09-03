package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/geo/r3"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	events "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

// Grenade type labels for annotation (烟/闪/雷/火/诱饵弹).
const (
	typeSmoke = "烟"
	typeFlash = "闪"
	typeHE    = "雷"
	typeFire  = "火"
	typeDecoy = "诱饵弹"
)

// utf8BOM lets Excel / WPS on Windows detect UTF-8 and show Chinese headers correctly.
const utf8BOM = "\uFEFF"

var csvHeader = []string{
	"道具所属地图",
	"道具所属demo",
	"道具种类",
	"道具投掷者",
	"起点X",
	"起点Y",
	"起点Z",
	"爆点X",
	"爆点Y",
	"爆点Z",
	"道具分类",
}

// GrenadeRecord is one thrown grenade, ready for CSV export / annotation.
type GrenadeRecord struct {
	Map         string
	DemoPath    string
	GrenadeType string
	Thrower     string
	Start       r3.Vector
	Detonate    r3.Vector
	Category    string
}

type options struct {
	dir  string
	demo string
	out  string
}

// Run like this:
//
//	go run . -dir /path/to/tournament -out grenades.csv
//	go run . -demo /path/to/demo.dem -out grenades.csv
func main() {
	opts, err := parseArgs(os.Args[1:])
	checkError(err)

	err = exportGrenades(opts, os.Stdout, os.Stderr)
	checkError(err)
}

func parseArgs(args []string) (options, error) {
	fs := flag.NewFlagSet("export-grenades", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	dir := fs.String("dir", "", "Tournament demo directory (match folders containing .dem files)")
	demo := fs.String("demo", "", "Single demo file path")
	out := fs.String("out", "", "Output CSV path (default: stdout)")

	err := fs.Parse(args)
	if err != nil {
		return options{}, err
	}

	opts := options{dir: *dir, demo: *demo, out: *out}
	if opts.dir == "" && opts.demo == "" {
		return options{}, fmt.Errorf("either -dir or -demo is required")
	}

	return opts, nil
}

func exportGrenades(opts options, stdout, stderr io.Writer) error {
	root := opts.dir
	if root == "" {
		root = opts.demo
	}

	demoPaths, err := collectDemoPaths(root)
	if err != nil {
		return err
	}

	if len(demoPaths) == 0 {
		return fmt.Errorf("no .dem files found under %s", root)
	}

	records := make([]GrenadeRecord, 0, len(demoPaths)*32)

	for i, demoPath := range demoPaths {
		fmt.Fprintf(stderr, "parsing %d/%d: %s\n", i+1, len(demoPaths), demoPath)

		demoRecords, parseErr := parseDemoGrenades(demoPath)
		if parseErr != nil {
			return fmt.Errorf("failed to parse %s: %w", demoPath, parseErr)
		}

		records = append(records, demoRecords...)
	}

	var out io.Writer = stdout

	if opts.out != "" {
		f, createErr := os.Create(opts.out)
		if createErr != nil {
			return fmt.Errorf("failed to create output file: %w", createErr)
		}
		defer f.Close()

		out = f
	}

	err = writeCSV(out, records)
	if err != nil {
		return err
	}

	fmt.Fprintf(stderr, "exported %d grenades from %d demos\n", len(records), len(demoPaths))

	return nil
}

func collectDemoPaths(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s: %w", root, err)
	}

	if !info.IsDir() {
		if !isDemoFile(root) {
			return nil, fmt.Errorf("not a .dem file: %s", root)
		}

		abs, absErr := filepath.Abs(root)
		if absErr != nil {
			return nil, absErr
		}

		return []string{abs}, nil
	}

	var paths []string

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() || !isDemoFile(path) {
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

	sort.Strings(paths)

	return paths, nil
}

func isDemoFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".dem")
}

func parseDemoGrenades(demoPath string) ([]GrenadeRecord, error) {
	var (
		mapName string
		records []GrenadeRecord
	)

	err := demoinfocs.ParseFile(demoPath, func(p demoinfocs.Parser) error {
		p.RegisterNetMessageHandler(func(m *msg.CDemoFileHeader) {
			if name := m.GetMapName(); name != "" {
				mapName = name
			}
		})

		p.RegisterNetMessageHandler(func(m *msg.CSVCMsg_ServerInfo) {
			if name := m.GetMapName(); name != "" {
				mapName = name
			}
		})

		p.RegisterEventHandler(func(e events.GrenadeProjectileDestroy) {
			rec, ok := recordFromProjectile(demoPath, mapName, e.Projectile)
			if !ok {
				return
			}

			records = append(records, rec)
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return records, nil
}

func recordFromProjectile(demoPath, mapName string, proj *common.GrenadeProjectile) (GrenadeRecord, bool) {
	if proj == nil || proj.WeaponInstance == nil {
		return GrenadeRecord{}, false
	}

	label, ok := grenadeTypeLabel(proj.WeaponInstance.Type)
	if !ok {
		return GrenadeRecord{}, false
	}

	start, detonate := trajectoryEndpoints(proj)

	thrower := ""
	if proj.Thrower != nil {
		thrower = proj.Thrower.Name
	}

	return GrenadeRecord{
		Map:         mapName,
		DemoPath:    demoPath,
		GrenadeType: label,
		Thrower:     thrower,
		Start:       start,
		Detonate:    detonate,
	}, true
}

func trajectoryEndpoints(proj *common.GrenadeProjectile) (start, detonate r3.Vector) {
	if len(proj.Trajectory) > 0 {
		start = proj.Trajectory[0].Position
		detonate = proj.Trajectory[len(proj.Trajectory)-1].Position

		return start, detonate
	}

	if proj.Entity != nil {
		detonate = proj.Position()
	}

	return start, detonate
}

func grenadeTypeLabel(t common.EquipmentType) (string, bool) {
	switch t {
	case common.EqSmoke:
		return typeSmoke, true
	case common.EqFlash:
		return typeFlash, true
	case common.EqHE:
		return typeHE, true
	case common.EqMolotov, common.EqIncendiary:
		return typeFire, true
	case common.EqDecoy:
		return typeDecoy, true
	default:
		return "", false
	}
}

func writeCSV(w io.Writer, records []GrenadeRecord) error {
	_, err := io.WriteString(w, utf8BOM)
	if err != nil {
		return fmt.Errorf("failed to write UTF-8 BOM: %w", err)
	}

	cw := csv.NewWriter(w)

	err = cw.Write(csvHeader)
	if err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	row := make([]string, len(csvHeader))

	for _, rec := range records {
		row[0] = rec.Map
		row[1] = rec.DemoPath
		row[2] = rec.GrenadeType
		row[3] = rec.Thrower
		row[4] = formatCoord(rec.Start.X)
		row[5] = formatCoord(rec.Start.Y)
		row[6] = formatCoord(rec.Start.Z)
		row[7] = formatCoord(rec.Detonate.X)
		row[8] = formatCoord(rec.Detonate.Y)
		row[9] = formatCoord(rec.Detonate.Z)
		row[10] = rec.Category

		err = cw.Write(row)
		if err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	cw.Flush()

	err = cw.Error()
	if err != nil {
		return fmt.Errorf("failed to flush CSV: %w", err)
	}

	return nil
}

func formatCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}
