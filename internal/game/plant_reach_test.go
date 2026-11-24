package game

import (
	"testing"

	"github.com/gigurra/counterslop/internal/content"
	"github.com/gigurra/counterslop/internal/content/maps"
)

// plants: a Vibe Coder carrying the Big Diff, dropped at 'at' on mapName,
// lands, draws it and holds fire for the plant time and a bit: planted?
func plants(t *testing.T, mapName string, at Vec3) bool {
	t.Helper()
	s := authServer(t)
	s.Map = maps.All[mapName]()
	s.cvars["mp_freezetime"] = "0"
	s.cvars["mp_warmup_time"] = "0"
	v, a := s.join("1.1.1.1"), s.join("2.2.2.2")
	v.ready, a.ready = true, true
	v.Team, a.Team = content.Vibe, content.Artisan
	s.queueCmds(v, []Cmd{{Seq: 1}})
	s.queueCmds(a, []Cmd{{Seq: 1}})
	now := s.now
	s.simTick(now)
	ticks(s, 2, &now, nil)
	if s.round.Bomb.State != BombCarried || s.round.Bomb.Carrier != v.ID {
		t.Fatalf("bomb %+v, want carried by the Vibe Coder", s.round.Bomb)
	}
	s.teleport(v, at, 0, false, v.lastQueued)
	ticks(s, 30, &now, map[*Player]Cmd{v: {}}) // land
	ticks(s, 2, &now, map[*Player]Cmd{v: {W: BombSlot}})
	ticks(s, int(plantSecs*SimHz)+10, &now, map[*Player]Cmd{v: {B: BtnFire}})
	return s.round.Bomb.State == BombPlanted
}

// Crate tops a boost gets a planter up to and no defender could get to the
// Big Diff on (2 m over where a defender stands: over the 1.5 m defuse reach;
// a crouch jump tops out at 1.16 m, nothing to step from): de_monorepo A's
// lone crate and the crate in the middle of de_slop A's platform. The plant
// is refused; on the floor beside the crate it goes in.
func TestPlantRefusedWhereNoDefuserReaches(t *testing.T) {
	for _, c := range []struct {
		m          string
		top, floor Vec3
	}{
		{"de_monorepo", Vec3{21.2, 2.0, 17.8}, Vec3{21.2, 0, 15}},
		{"de_slop", Vec3{23, 3.5, 23}, Vec3{19, 1.5, 22.5}},
	} {
		if plants(t, c.m, c.top) {
			t.Errorf("%s: planted on the crate top at %s: no defender can reach it", c.m, v3(c.top))
		}
		if !plants(t, c.m, c.floor) {
			t.Errorf("%s: no plant on the floor at %s", c.m, v3(c.floor))
		}
	}
}
