package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/golang/geo/r3"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

type Vec3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type ViewAngles struct {
	Yaw   float32 `json:"yaw"`   // ViewDirectionX
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
	GroundSpeed     float64     `json:"ground_speed"` // approximate cl_showpos vel (XY units/s)
	Airborne        bool        `json:"airborne"`
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

type playerMotionSample struct {
	pos     r3.Vector
	tick    int
	hasPrev bool
	speed   float64 // last computed ground speed
}

type exporter struct {
	mapName  string
	round    int
	tickRate float64

	motion  map[uint64]playerMotionSample
	buttons map[uint64][]timedButtons
	pending map[int64]*pendingThrow
	byRound map[int][]GrenadeThrow
}

func newExporter() *exporter {
	return &exporter{
		motion:  make(map[uint64]playerMotionSample),
		buttons: make(map[uint64][]timedButtons),
		pending: make(map[int64]*pendingThrow),
		byRound: make(map[int][]GrenadeThrow),
	}
}

const maxButtonSamples = 1024

func playerKey(pl *common.Player) uint64 {
	if pl == nil {
		return 0
	}
	if pl.SteamID64 != 0 {
		return pl.SteamID64
	}
	return uint64(uint32(pl.EntityID))<<32 | uint64(uint32(pl.UserID))
}

func (e *exporter) recordButtons(key uint64, tick int, state uint64) {
	if key == 0 {
		return
	}
	hist := e.buttons[key]
	if n := len(hist); n > 0 {
		if hist[n-1].tick == tick && hist[n-1].state == state {
			return
		}
		if hist[n-1].state == state {
			hist[n-1].tick = tick
			e.buttons[key] = hist
			return
		}
	}
	hist = append(hist, timedButtons{tick: tick, state: state})
	if len(hist) > maxButtonSamples {
		hist = hist[len(hist)-maxButtonSamples:]
	}
	e.buttons[key] = hist
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

func groundSpeedBetween(prev, cur r3.Vector, prevTick, curTick int, tickRate float64) (float64, bool) {
	if tickRate <= 0 || curTick <= prevTick {
		return 0, false
	}
	dt := float64(curTick-prevTick) / tickRate
	if dt <= 0 {
		return 0, false
	}
	return math.Hypot(cur.X-prev.X, cur.Y-prev.Y) / dt, true
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

func (e *exporter) trackPlayers(gs demoinfocs.GameState, tickRate float64) {
	if tickRate > 0 {
		e.tickRate = tickRate
	}
	tick := gs.IngameTick()
	for _, pl := range gs.Participants().Playing() {
		if pl == nil {
			continue
		}
		key := playerKey(pl)
		if key == 0 {
			continue
		}
		pos := pl.Position()
		sample := e.motion[key]
		if sample.hasPrev {
			if speed, ok := groundSpeedBetween(sample.pos, pos, sample.tick, tick, e.tickRate); ok {
				sample.speed = speed
			}
		}
		sample.pos = pos
		sample.tick = tick
		sample.hasPrev = true
		e.motion[key] = sample
		e.recordButtons(key, tick, pl.ButtonsPressedState)
	}
}

func (e *exporter) groundSpeedAt(key uint64, pos r3.Vector, tick int) float64 {
	sample, ok := e.motion[key]
	if !ok || !sample.hasPrev {
		return 0
	}
	if speed, ok := groundSpeedBetween(sample.pos, pos, sample.tick, tick, e.tickRate); ok {
		return speed
	}
	// Same tick as last sample: reuse last frame speed.
	return sample.speed
}

func (e *exporter) onRoundStart(gs demoinfocs.GameState) {
	if gs.IsWarmupPeriod() {
		return
	}
	n := gs.TotalRoundsPlayed() + 1
	if n < 1 {
		n = 1
	}
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
		throwerPos  r3.Vector
		view        ViewAngles
		airborne    bool
		groundSpeed float64
		throwMethod string
	)

	if pl := proj.Thrower; pl != nil {
		key := playerKey(pl)
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
		tick := gs.IngameTick()
		groundSpeed = e.groundSpeedAt(key, throwerPos, tick)
		e.recordButtons(key, tick, pl.ButtonsPressedState)
		attack, attack2 := lastGrenadeClickHold(e.buttons[key], tick)
		forward, back, left, right := lastDirectionHold(e.buttons[key], tick)
		in := throwClassificationInput{
			LeftClickHeld:  attack,
			RightClickHeld: attack2,
			Forward:        forward,
			Back:           back,
			Left:           left,
			Right:          right,
			Duck:           pl.IsPressingButton(common.ButtonDuck),
			Walk:           pl.IsPressingButton(common.ButtonSpeed),
			Airborne:       airborne,
			GroundSpeed:    groundSpeed,
		}
		throwMethod = formatThrowMethod(in)
	} else {
		throwMethod = formatThrowMethod(throwClassificationInput{})
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
			GroundSpeed:     groundSpeed,
			Airborne:        airborne,
			ThrowMethod:     throwMethod,
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
		exp.trackPlayers(parser.GameState(), parser.TickRate())
	})

	parser.RegisterEventHandler(func(ev events.PlayerButtonsStateUpdate) {
		if ev.Player == nil {
			return
		}
		exp.recordButtons(playerKey(ev.Player), parser.GameState().IngameTick(), ev.ButtonsState)
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
