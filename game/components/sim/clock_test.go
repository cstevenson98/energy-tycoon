package sim_test

import (
	"testing"
	"time"

	"github.com/cstevenson98/energy-tycoon/game/components/sim"
)

func TestFormatSimTime(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{sim.EpochMs, "1 Jan 2027 00:00:00"},
		{sim.EpochMs + sim.MsPerHour + 2*sim.MsPerMinute + 3*sim.MsPerSecond, "1 Jan 2027 01:02:03"},
		{sim.EpochMs + sim.MsPerDay, "2 Jan 2027 00:00:00"},
		{sim.EpochMs + 30*sim.MsPerDay + 13*sim.MsPerHour, "31 Jan 2027 13:00:00"},
	}
	for _, tc := range cases {
		if got := sim.FormatSimTime(tc.ms); got != tc.want {
			t.Errorf("FormatSimTime(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestNewSimClockStartsAtEpoch(t *testing.T) {
	c := sim.NewSimClock()
	if c.NowMs != sim.EpochMs {
		t.Fatalf("NowMs = %d, want EpochMs %d (%s)", c.NowMs, sim.EpochMs, time.UnixMilli(sim.EpochMs).UTC())
	}
	if c.SpeedIndex != sim.DefaultSpeedIndex {
		t.Fatalf("SpeedIndex = %d, want %d", c.SpeedIndex, sim.DefaultSpeedIndex)
	}
	if got := c.SpeedMsPerRealSec(); got != sim.MsPerHour {
		t.Fatalf("SpeedMsPerRealSec = %d, want %d", got, sim.MsPerHour)
	}
}

func TestSetSpeedIndexClamps(t *testing.T) {
	c := sim.NewSimClock()
	c.SetSpeedIndex(-1)
	if c.SpeedIndex != 0 {
		t.Fatalf("clamp low: got %d", c.SpeedIndex)
	}
	c.SetSpeedIndex(99)
	if c.SpeedIndex != sim.NumSpeeds-1 {
		t.Fatalf("clamp high: got %d", c.SpeedIndex)
	}
	if c.SpeedMsPerRealSec() != sim.MsPerWeek {
		t.Fatalf("fastest rate = %d, want %d", c.SpeedMsPerRealSec(), sim.MsPerWeek)
	}
}
