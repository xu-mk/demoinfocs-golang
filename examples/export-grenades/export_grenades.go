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
	"sort"
	"strconv"
	"strings"

	"github.com/golang/geo/r3"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	events "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

const (
	typeSmoke      = "smoke"
	typeFlash      = "flash"
	typeHE         = "he"
	typeMolotov    = "molotov"
	typeIncendiary = "incendiary"
	typeDecoy      = "decoy"

	colDemo       = "demo"
	colDemoLegacy = "道具所属demo"
)

// utf8BOM lets Excel / WPS on Windows detect UTF-8.
const utf8BOM = "\uFEFF"

var csvHeader = []string{
	"map",
	"demo",
	"grenade_type",
	"thrower",
	"start_x",
	"start_y",
	"start_z",
	"view_x",
	"view_y",
	"detonate_x",
	"detonate_y",
	"detonate_z",
	"setpos/setang",
	"gen_grenade_explode",
	"category",
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
	dir       string
	demo      string
	out       string
	overwrite bool
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
	overwrite := fs.Bool("overwrite", false, "Overwrite existing CSV instead of resuming")

	err := fs.Parse(args)
	if err != nil {
		return options{}, err
	}

	opts := options{dir: *dir, demo: *demo, out: *out, overwrite: *overwrite}
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

	sink, done, closer, err := setupOutput(opts, stdout)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer()
	}

	var (
		nGrenades int
		nParsed   int
		nSkipped  int
		nFailed   int
	)

	for i, demo := range demos {
		if _, ok := done[demo.rel]; ok {
			nSkipped++
			fmt.Fprintf(stderr, "skip %d/%d (already exported): %s\n", i+1, len(demos), demo.rel)

			continue
		}

		fmt.Fprintf(stderr, "parsing %d/%d: %s\n", i+1, len(demos), demo.rel)

		demoRecords, parseErr := parseDemoGrenades(demo.abs, demo.rel)
		if parseErr != nil {
			nFailed++
			fmt.Fprintf(stderr, "error %d/%d %s: %v\n", i+1, len(demos), demo.rel, parseErr)

			continue
		}

		writeErr := sink.writeRecords(demoRecords)
		if writeErr != nil {
			return writeErr
		}

		if opts.out != "" {
			progressErr := appendProgress(progressPath(opts.out), demo.rel)
			if progressErr != nil {
				return progressErr
			}
		}

		done[demo.rel] = struct{}{}
		nGrenades += len(demoRecords)
		nParsed++
	}

	fmt.Fprintf(stderr, "exported %d grenades from %d demos (skipped %d, failed %d)\n", nGrenades, nParsed, nSkipped, nFailed)

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

func genGrenadeExplode(rec GrenadeRecord) string {
	return "gen_grenade_explode " + rec.GrenadeType + " " +
		formatCoord(rec.Detonate.X) + " " + formatCoord(rec.Detonate.Y) + " " + formatCoord(rec.Detonate.Z)
}

func grenadeTypeLabel(t common.EquipmentType) (string, bool) {
	switch t {
	case common.EqSmoke:
		return typeSmoke, true
	case common.EqFlash:
		return typeFlash, true
	case common.EqHE:
		return typeHE, true
	case common.EqMolotov:
		return typeMolotov, true
	case common.EqIncendiary:
		return typeIncendiary, true
	case common.EqDecoy:
		return typeDecoy, true
	default:
		return "", false
	}
}

func progressPath(outPath string) string {
	return outPath + ".progress"
}

func setupOutput(opts options, stdout io.Writer) (*csvSink, map[string]struct{}, func() error, error) {
	if opts.out == "" {
		sink, err := newCSVSink(stdout, true)
		return sink, map[string]struct{}{}, nil, err
	}

	if opts.overwrite {
		_ = os.Remove(progressPath(opts.out))
	}

	info, statErr := os.Stat(opts.out)
	exists := statErr == nil && info.Size() > 0

	if exists && !opts.overwrite {
		done, loadErr := loadCompletedDemos(opts.out, progressPath(opts.out))
		if loadErr != nil {
			return nil, nil, nil, loadErr
		}

		f, openErr := os.OpenFile(opts.out, os.O_APPEND|os.O_WRONLY, 0o644)
		if openErr != nil {
			return nil, nil, nil, fmt.Errorf("failed to open output file: %w", openErr)
		}

		sink, sinkErr := newCSVSink(f, false)
		if sinkErr != nil {
			f.Close()
			return nil, nil, nil, sinkErr
		}

		return sink, done, f.Close, nil
	}

	f, createErr := os.Create(opts.out)
	if createErr != nil {
		return nil, nil, nil, fmt.Errorf("failed to create output file: %w", createErr)
	}

	sink, sinkErr := newCSVSink(f, true)
	if sinkErr != nil {
		f.Close()
		return nil, nil, nil, sinkErr
	}

	return sink, map[string]struct{}{}, f.Close, nil
}

func loadCompletedDemos(csvPath, progressFile string) (map[string]struct{}, error) {
	done := make(map[string]struct{})

	if err := loadCompletedFromCSV(csvPath, done); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if err := loadCompletedFromProgress(progressFile, done); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return done, nil
}

func loadCompletedFromCSV(path string, done map[string]struct{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(stripBOM(f))
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil
		}

		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	demoCol := indexOf(header, colDemo)
	if demoCol < 0 {
		demoCol = indexOf(header, colDemoLegacy)
	}
	if demoCol < 0 {
		return nil
	}

	for {
		row, readErr := r.Read()
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("failed to read CSV: %w", readErr)
		}

		if demoCol < len(row) && row[demoCol] != "" {
			done[row[demoCol]] = struct{}{}
		}
	}
}

func loadCompletedFromProgress(path string, done map[string]struct{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			done[line] = struct{}{}
		}
	}

	return nil
}

func appendProgress(path, rel string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to update progress file: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, rel)
	if err != nil {
		return fmt.Errorf("failed to write progress file: %w", err)
	}

	return f.Sync()
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

type csvSink struct {
	cw   *csv.Writer
	sync func() error
	row  []string
}

func newCSVSink(w io.Writer, writeHeader bool) (*csvSink, error) {
	if writeHeader {
		_, err := io.WriteString(w, utf8BOM)
		if err != nil {
			return nil, fmt.Errorf("failed to write UTF-8 BOM: %w", err)
		}
	}

	sink := &csvSink{
		cw:  csv.NewWriter(w),
		row: make([]string, len(csvHeader)),
	}

	if f, ok := w.(*os.File); ok {
		sink.sync = f.Sync
	}

	if writeHeader {
		err := sink.cw.Write(csvHeader)
		if err != nil {
			return nil, fmt.Errorf("failed to write CSV header: %w", err)
		}

		err = sink.flush()
		if err != nil {
			return nil, err
		}
	}

	return sink, nil
}

func (s *csvSink) writeRecords(records []GrenadeRecord) error {
	for _, rec := range records {
		s.row[0] = rec.Map
		s.row[1] = rec.DemoPath
		s.row[2] = rec.GrenadeType
		s.row[3] = rec.Thrower
		s.row[4] = formatCoord(rec.Start.X)
		s.row[5] = formatCoord(rec.Start.Y)
		s.row[6] = formatCoord(rec.Start.Z)
		s.row[7] = formatCoord(rec.ViewAngles.X)
		s.row[8] = formatCoord(rec.ViewAngles.Y)
		s.row[9] = formatCoord(rec.Detonate.X)
		s.row[10] = formatCoord(rec.Detonate.Y)
		s.row[11] = formatCoord(rec.Detonate.Z)
		s.row[12] = copyPayload(rec)
		s.row[13] = genGrenadeExplode(rec)
		s.row[14] = rec.Category

		err := s.cw.Write(s.row)
		if err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return s.flush()
}

func (s *csvSink) flush() error {
	s.cw.Flush()

	err := s.cw.Error()
	if err != nil {
		return fmt.Errorf("failed to flush CSV: %w", err)
	}

	if s.sync != nil {
		err = s.sync()
		if err != nil {
			return fmt.Errorf("failed to sync CSV: %w", err)
		}
	}

	return nil
}

func writeCSV(w io.Writer, records []GrenadeRecord) error {
	sink, err := newCSVSink(w, true)
	if err != nil {
		return err
	}

	return sink.writeRecords(records)
}

func formatCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}
