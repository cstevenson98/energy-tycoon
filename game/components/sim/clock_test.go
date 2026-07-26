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

func TestHoursSinceEpoch(t *testing.T) {
	if got := sim.HoursSinceEpoch(sim.EpochMs); got != 0 {
		t.Fatalf("HoursSinceEpoch(epoch) = %v, want 0", got)
	}
	if got := sim.HoursSinceEpoch(sim.EpochMs + 90*sim.MsPerMinute); got != 1.5 {
		t.Fatalf("HoursSinceEpoch(90m) = %v, want 1.5", got)
	}
}

func TestHourOfDay(t *testing.T) {
	if got := sim.HourOfDay(sim.EpochMs); got != 0 {
		t.Fatalf("HourOfDay(midnight) = %v, want 0", got)
	}
	if got := sim.HourOfDay(sim.EpochMs + 13*sim.MsPerHour + 30*sim.MsPerMinute); got != 13.5 {
		t.Fatalf("HourOfDay(13:30) = %v, want 13.5", got)
	}
	if got := sim.HourOfDay(sim.EpochMs + 25*sim.MsPerHour); got != 1 {
		t.Fatalf("HourOfDay(next day 01:00) = %v, want 1", got)
	}
}

func TestFormatClockHM(t *testing.T) {
	ms := sim.EpochMs + 15*sim.MsPerHour + 30*sim.MsPerMinute
	if got := sim.FormatClockHM(ms); got != "15:30" {
		t.Fatalf("FormatClockHM = %q, want 15:30", got)
	}
	if sim.MsFromHoursSinceEpoch(15.5) != ms {
		t.Fatalf("MsFromHoursSinceEpoch(15.5) = %d, want %d", sim.MsFromHoursSinceEpoch(15.5), ms)
	}
}

func TestDayFraction(t *testing.T) {
	cases := []struct {
		ms   int64
		want float64
	}{
		{sim.EpochMs, 0},
		{sim.EpochMs + 12*sim.MsPerHour, 0.5},
		{sim.EpochMs + sim.MsPerDay - 1, float64(sim.MsPerDay-1) / float64(sim.MsPerDay)},
		{sim.EpochMs + sim.MsPerDay, 0},
		{sim.EpochMs + 6*sim.MsPerHour, 0.25},
	}
	for _, tc := range cases {
		got := sim.DayFraction(tc.ms)
		if got != tc.want {
			t.Errorf("DayFraction(%d) = %v, want %v", tc.ms, got, tc.want)
		}
	}
}
