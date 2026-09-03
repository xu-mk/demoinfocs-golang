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
	"准星角度X",
	"准星角度Y",
	"爆点X",
	"爆点Y",
	"爆点Z",
	"道具分类",
	"setpos/setang",
}

// GrenadeRecord is one thrown grenade, ready for CSV export / annotation.
type GrenadeRecord struct {
	Map         string
	DemoPath    string
	GrenadeType string
	Thrower     string
	Start       r3.Vector // thrower position at throw time
	ViewAngles  r3.Vector // eye angles (pitch/yaw/roll) at throw time
	Detonate    r3.Vector
	Category    string
}

type throwSnapshot struct {
	pos    r3.Vector
	angles r3.Vector
	ok     bool
}

type demoRef struct {
	abs string
	rel string
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

	demos, err := collectDemoPaths(root)
	if err != nil {
		return err
	}

	if len(demos) == 0 {
		return fmt.Errorf("no .dem files found under %s", root)
	}

	records := make([]GrenadeRecord, 0, len(demos)*32)

	for i, demo := range demos {
		fmt.Fprintf(stderr, "parsing %d/%d: %s\n", i+1, len(demos), demo.rel)

		demoRecords, parseErr := parseDemoGrenades(demo.abs, demo.rel)
		if parseErr != nil {
			return fmt.Errorf("failed to parse %s: %w", demo.rel, parseErr)
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

	fmt.Fprintf(stderr, "exported %d grenades from %d demos\n", len(records), len(demos))

	return nil
}

func collectDemoPaths(root string) ([]demoRef, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s: %w", root, err)
	}

	if !info.IsDir() {
		if !isDemoFile(root) {
			return nil, fmt.Errorf("not a .dem file: %s", root)
		}

		ref, refErr := newDemoRef(root, root)
		if refErr != nil {
			return nil, refErr
		}

		return []demoRef{ref}, nil
	}

	var refs []demoRef

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() || !isDemoFile(path) {
			return nil
		}

		ref, refErr := newDemoRef(root, path)
		if refErr != nil {
			return refErr
		}

		refs = append(refs, ref)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk %s: %w", root, err)
	}

	sort.Slice(refs, func(i, j int) bool {
		return refs[i].rel < refs[j].rel
	})

	return refs, nil
}

func newDemoRef(root, path string) (demoRef, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return demoRef{}, err
	}

	rel, err := relativeDemoPath(root, abs)
	if err != nil {
		return demoRef{}, err
	}

	return demoRef{abs: abs, rel: rel}, nil
}

func relativeDemoPath(root, absPath string) (string, error) {
	info, err := os.Stat(root)
	base := root
	if err == nil && !info.IsDir() {
		base = filepath.Dir(root)
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return filepath.ToSlash(absPath), nil
	}

	return filepath.ToSlash(rel), nil
}

func isDemoFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".dem")
}

func parseDemoGrenades(absPath, relPath string) ([]GrenadeRecord, error) {
	var (
		mapName string
		records []GrenadeRecord
	)

	throws := make(map[int64]throwSnapshot)

	err := demoinfocs.ParseFile(absPath, func(p demoinfocs.Parser) error {
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

		p.RegisterEventHandler(func(e events.GrenadeProjectileThrow) {
			if e.Projectile == nil {
				return
			}

			throws[e.Projectile.UniqueID()] = captureThrowSnapshot(e.Projectile.Thrower)
		})

		p.RegisterEventHandler(func(e events.GrenadeProjectileDestroy) {
			var snap throwSnapshot
			if e.Projectile != nil {
				snap = throws[e.Projectile.UniqueID()]
			}

			rec, ok := recordFromProjectile(relPath, mapName, e.Projectile, snap)
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

func recordFromProjectile(demoPath, mapName string, proj *common.GrenadeProjectile, snap throwSnapshot) (GrenadeRecord, bool) {
	if proj == nil || proj.WeaponInstance == nil {
		return GrenadeRecord{}, false
	}

	label, ok := grenadeTypeLabel(proj.WeaponInstance.Type)
	if !ok {
		return GrenadeRecord{}, false
	}

	thrower := ""
	if proj.Thrower != nil {
		thrower = proj.Thrower.Name
	}

	start, angles := throwerPose(proj.Thrower, snap)

	return GrenadeRecord{
		Map:         mapName,
		DemoPath:    demoPath,
		GrenadeType: label,
		Thrower:     thrower,
		Start:       start,
		ViewAngles:  angles,
		Detonate:    detonatePosition(proj),
	}, true
}

func throwerPose(thrower *common.Player, snap throwSnapshot) (pos, angles r3.Vector) {
	if snap.ok {
		return snap.pos, snap.angles
	}

	if thrower == nil {
		return r3.Vector{}, r3.Vector{}
	}

	fallback := captureThrowSnapshot(thrower)

	return fallback.pos, fallback.angles
}

func captureThrowSnapshot(thrower *common.Player) throwSnapshot {
	if thrower == nil {
		return throwSnapshot{}
	}

	snap := throwSnapshot{
		ok:  true,
		pos: thrower.Position(),
		angles: r3.Vector{
			X: float64(thrower.ViewDirectionY()),
			Y: float64(thrower.ViewDirectionX()),
		},
	}

	if pawn := thrower.PlayerPawnEntity(); pawn != nil {
		if val, ok := pawn.PropertyValue("m_angEyeAngles"); ok && val.Any != nil {
			snap.angles = val.R3Vec()
		}
	}

	return snap
}

func detonatePosition(proj *common.GrenadeProjectile) r3.Vector {
	if len(proj.Trajectory) > 0 {
		return proj.Trajectory[len(proj.Trajectory)-1].Position
	}

	if proj.Entity != nil {
		return proj.Position()
	}

	return r3.Vector{}
}

func copyPayload(rec GrenadeRecord) string {
	return "setpos " + formatCoord(rec.Start.X) + " " + formatCoord(rec.Start.Y) + " " + formatCoord(rec.Start.Z) +
		"; setang " + formatCoord(rec.ViewAngles.X) + " " + formatCoord(rec.ViewAngles.Y)
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
		row[7] = formatCoord(rec.ViewAngles.X)
		row[8] = formatCoord(rec.ViewAngles.Y)
		row[9] = formatCoord(rec.Detonate.X)
		row[10] = formatCoord(rec.Detonate.Y)
		row[11] = formatCoord(rec.Detonate.Z)
		row[12] = rec.Category
		row[13] = copyPayload(rec)

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
