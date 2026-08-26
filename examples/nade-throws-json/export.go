package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/golang/geo/r3"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

const playerPositionHistorySize = 24 // ~0.375s at 64-tick; enough to see jump ascent / run-up

type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type ViewAngles struct {
	Yaw   float32 `json:"yaw"`   // ViewDirectionX, 0..360
	Pitch float32 `json:"pitch"` // ViewDirectionY
}

type ThrowerInfo struct {
	Name      string `json:"name"`
	SteamID64 uint64 `json:"steam_id_64"`
	UserID    int    `json:"user_id"`
	Team      string `json:"team"`
}

type GrenadeThrow struct {
	Type            string      `json:"type"`
	ID              int64       `json:"id"`
	Thrower         ThrowerInfo `json:"thrower"`
	ThrowerPosition Vec3        `json:"thrower_position"`
	ThrowerView     ViewAngles  `json:"thrower_view_angles"`
	Start           Vec3        `json:"start"`
	End             Vec3        `json:"end"`
	ThrowMethod     string      `json:"throw_method"`
	Tick            int         `json:"tick"`
}

type RoundThrows struct {
	Round    int            `json:"round"`
	Grenades []GrenadeThrow `json:"grenades"`
}

type DemoThrowsJSON struct {
	Map    string        `json:"map"`
	Rounds []RoundThrows `json:"rounds"`
}

type pendingThrow struct {
	grenade GrenadeThrow
	round   int
}

type exporter struct {
	mapName string
	round   int

	// steamID64 -> recent positions (oldest -> newest)
	posHistory map[uint64][]r3.Vector

	// projectile UniqueID -> incomplete throw
	pending map[int64]*pendingThrow

	// round number -> throws
	byRound map[int][]GrenadeThrow
}

func newExporter() *exporter {
	return &exporter{
		posHistory: make(map[uint64][]r3.Vector),
		pending:    make(map[int64]*pendingThrow),
		byRound:    make(map[int][]GrenadeThrow),
	}
}

func teamName(t common.Team) string {
	switch t {
	case common.TeamTerrorists:
		return "T"
	case common.TeamCounterTerrorists:
		return "CT"
	case common.TeamSpectators:
		return "SPECTATOR"
	default:
		return "UNASSIGNED"
	}
}

func toVec3(v r3.Vector) Vec3 {
	return Vec3{X: v.X, Y: v.Y, Z: v.Z}
}

func (e *exporter) currentRound(gs demoinfocs.GameState) int {
	if e.round > 0 {
		return e.round
	}
	n := gs.TotalRoundsPlayed() + 1
	if n < 1 {
		return 1
	}
	return n
}

func (e *exporter) trackPlayers(gs demoinfocs.GameState) {
	for _, pl := range gs.Participants().Playing() {
		if pl == nil || pl.SteamID64 == 0 {
			continue
		}
		pos := pl.Position()
		hist := append(e.posHistory[pl.SteamID64], pos)
		if len(hist) > playerPositionHistorySize {
			hist = hist[len(hist)-playerPositionHistorySize:]
		}
		e.posHistory[pl.SteamID64] = hist
	}
}

func (e *exporter) onRoundStart(gs demoinfocs.GameState) {
	if gs.IsWarmupPeriod() {
		return
	}
	n := gs.TotalRoundsPlayed() + 1
	if n < 1 {
		n = 1
	}
	// Prefer CCSGameRules' total rounds. Ignore duplicate RoundStart for the same value.
	if n > e.round {
		e.round = n
	} else if e.round == 0 {
		e.round = 1
	}
}

func (e *exporter) onThrow(gs demoinfocs.GameState, ev events.GrenadeProjectileThrow) {
	proj := ev.Projectile
	if proj == nil || proj.WeaponInstance == nil {
		return
	}
	if gs.IsWarmupPeriod() {
		return
	}

	round := e.currentRound(gs)
	e.round = round

	throwerInfo := ThrowerInfo{Name: "unknown"}
	var (
		throwerPos r3.Vector
		view       ViewAngles
		airborne   bool
		posHistory []r3.Vector
	)

	if pl := proj.Thrower; pl != nil {
		throwerInfo = ThrowerInfo{
			Name:      pl.Name,
			SteamID64: pl.SteamID64,
			UserID:    pl.UserID,
			Team:      teamName(pl.Team),
		}
		throwerPos = pl.Position()
		view = ViewAngles{
			Yaw:   pl.ViewDirectionX(),
			Pitch: pl.ViewDirectionY(),
		}
		airborne = pl.IsAirborne()
		posHistory = append([]r3.Vector(nil), e.posHistory[pl.SteamID64]...)
		if len(posHistory) == 0 || posHistory[len(posHistory)-1] != throwerPos {
			posHistory = append(posHistory, throwerPos)
		}
	}

	start := proj.Position()
	if len(proj.Trajectory) > 0 {
		start = proj.Trajectory[0].Position
	}

	id := proj.UniqueID()
	e.pending[id] = &pendingThrow{
		round: round,
		grenade: GrenadeThrow{
			Type:            proj.WeaponInstance.Type.String(),
			ID:              id,
			Thrower:         throwerInfo,
			ThrowerPosition: toVec3(throwerPos),
			ThrowerView:     view,
			Start:           toVec3(start),
			ThrowMethod:     classifyThrowMethod(posHistory, airborne),
			Tick:            gs.IngameTick(),
		},
	}
}

func (e *exporter) onDestroy(ev events.GrenadeProjectileDestroy) {
	proj := ev.Projectile
	if proj == nil {
		return
	}

	id := proj.UniqueID()
	pending, ok := e.pending[id]
	if !ok {
		return
	}

	end := proj.Position()
	if n := len(proj.Trajectory); n > 0 {
		end = proj.Trajectory[n-1].Position
	}
	pending.grenade.End = toVec3(end)

	round := pending.round
	if round < 1 {
		round = 1
	}
	e.byRound[round] = append(e.byRound[round], pending.grenade)
	delete(e.pending, id)
}

func (e *exporter) finalize() DemoThrowsJSON {
	for id, p := range e.pending {
		g := p.grenade
		g.End = g.Start
		round := p.round
		if round < 1 {
			round = e.round
			if round < 1 {
				round = 1
			}
		}
		e.byRound[round] = append(e.byRound[round], g)
		delete(e.pending, id)
	}

	maxRound := e.round
	for r := range e.byRound {
		if r > maxRound {
			maxRound = r
		}
	}

	rounds := make([]RoundThrows, 0, maxRound)
	for r := 1; r <= maxRound; r++ {
		grenades := e.byRound[r]
		if grenades == nil {
			grenades = []GrenadeThrow{}
		}
		rounds = append(rounds, RoundThrows{
			Round:    r,
			Grenades: grenades,
		})
	}

	return DemoThrowsJSON{
		Map:    e.mapName,
		Rounds: rounds,
	}
}

// ExportDemoThrows parses a CS2 demo and returns per-round grenade throw JSON data.
func ExportDemoThrows(r io.Reader) (DemoThrowsJSON, error) {
	parser := demoinfocs.NewParser(r)
	defer parser.Close()

	exp := newExporter()

	parser.RegisterNetMessageHandler(func(m *msg.CSVCMsg_ServerInfo) {
		exp.mapName = m.GetMapName()
	})

	parser.RegisterEventHandler(func(events.RoundStart) {
		exp.onRoundStart(parser.GameState())
	})

	parser.RegisterEventHandler(func(events.FrameDone) {
		exp.trackPlayers(parser.GameState())
	})

	parser.RegisterEventHandler(func(ev events.GrenadeProjectileThrow) {
		exp.onThrow(parser.GameState(), ev)
	})

	parser.RegisterEventHandler(func(ev events.GrenadeProjectileDestroy) {
		exp.onDestroy(ev)
	})

	if err := parser.ParseToEnd(); err != nil {
		return DemoThrowsJSON{}, fmt.Errorf("parse demo: %w", err)
	}

	return exp.finalize(), nil
}

// ExportDemoThrowsFile opens path and exports throw JSON.
func ExportDemoThrowsFile(path string) (DemoThrowsJSON, error) {
	f, err := os.Open(path)
	if err != nil {
		return DemoThrowsJSON{}, err
	}
	defer f.Close()
	return ExportDemoThrows(f)
}

func writeJSON(w io.Writer, data DemoThrowsJSON, pretty bool) error {
	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(data)
}
