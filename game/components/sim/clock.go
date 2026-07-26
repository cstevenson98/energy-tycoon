// Package sim holds simulation-time resources shared across systems.
package sim

import "time"

const (
	MsPerSecond = int64(1000)
	MsPerMinute = 60 * MsPerSecond
	MsPerHour   = 60 * MsPerMinute
	MsPerDay    = 24 * MsPerHour
	MsPerWeek   = 7 * MsPerDay

	DefaultSpeedIndex = 2 // 1 sim-hour per real second
	NumSpeeds         = 5
)

// EpochMs is sim t=0: 1 January 2027 00:00:00 UTC, as Unix milliseconds.
var EpochMs = time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

// SpeedLevels is sim-ms advanced per 1 real second for each speed index.
var SpeedLevels = [NumSpeeds]int64{
	5 * MsPerMinute,  // 5m/s
	15 * MsPerMinute, // 15m/s
	MsPerHour,        // 1h/s (default)
	MsPerDay,         // 1d/s
	MsPerWeek,        // 1w/s (fastest)
}

// SpeedLabels are short UI labels for SpeedLevels.
var SpeedLabels = [NumSpeeds]string{"5m", "15m", "1h", "1d", "1w"}

// SimClock is the world resource for pause/play and scaled simulation time.
// NowMs is an absolute Unix-ms timestamp on the sim calendar (epoch = 1 Jan 2027).
type SimClock struct {
	NowMs      int64 // absolute sim time (Unix ms)
	DeltaMs    int64 // sim ms advanced this frame (0 if paused)
	Playing    bool
	SpeedIndex int // 0..NumSpeeds-1
}

// NewSimClock returns a playing clock at the default speed (1h/s), starting
// at EpochMs (1 Jan 2027 00:00 UTC).
func NewSimClock() *SimClock {
	return &SimClock{
		NowMs:      EpochMs,
		Playing:    true,
		SpeedIndex: DefaultSpeedIndex,
	}
}

// ClampSpeedIndex returns idx forced into [0, NumSpeeds).
func ClampSpeedIndex(idx int) int {
	if idx < 0 {
		return 0
	}
	if idx >= NumSpeeds {
		return NumSpeeds - 1
	}
	return idx
}

// SetSpeedIndex sets SpeedIndex, clamped to a valid level.
func (c *SimClock) SetSpeedIndex(idx int) {
	if c == nil {
		return
	}
	c.SpeedIndex = ClampSpeedIndex(idx)
}

// SpeedMsPerRealSec returns how many sim-ms advance per real second at the
// current speed. Nil / out-of-range indices fall back to the default level.
func (c *SimClock) SpeedMsPerRealSec() int64 {
	if c == nil {
		return SpeedLevels[DefaultSpeedIndex]
	}
	return SpeedLevels[ClampSpeedIndex(c.SpeedIndex)]
}

// FormatSimTime renders an absolute Unix-ms sim timestamp as a calendar date.
func FormatSimTime(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2 Jan 2006 15:04:05")
}
