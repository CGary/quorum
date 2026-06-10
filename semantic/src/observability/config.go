package observability

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Level string

const (
	OffLevel   Level = "off"
	BasicLevel Level = "basic"
	DebugLevel Level = "debug"
	TraceLevel Level = "trace"
)

type Config struct {
	Level               Level
	DefaultSampleRate   float64
	SlowThresholds      map[string]time.Duration
	RawRetentionDays    int
	MinuteRetentionDays int
	HourRetentionDays   int
	DayRetentionDays    int
	FlushInterval       time.Duration
}

func LoadConfigFromEnv() Config {
	cfg := Config{
		Level:               parseLevel(os.Getenv("HSME_OBS_LEVEL")),
		DefaultSampleRate:   parseFloat(os.Getenv("HSME_OBS_SAMPLE_RATE"), 0.10),
		SlowThresholds:      parseThresholds(os.Getenv("HSME_OBS_SLOW_THRESHOLDS")),
		RawRetentionDays:    parseInt(os.Getenv("HSME_OBS_RAW_RETENTION_DAYS"), 7),
		MinuteRetentionDays: parseInt(os.Getenv("HSME_OBS_MINUTE_RETENTION_DAYS"), 7),
		HourRetentionDays:   parseInt(os.Getenv("HSME_OBS_HOUR_RETENTION_DAYS"), 30),
		DayRetentionDays:    parseInt(os.Getenv("HSME_OBS_DAY_RETENTION_DAYS"), 365),
		FlushInterval:       time.Duration(parseInt(os.Getenv("HSME_OBS_FLUSH_INTERVAL_SECONDS"), 60)) * time.Second,
	}
	if len(cfg.SlowThresholds) == 0 {
		cfg.SlowThresholds = map[string]time.Duration{
			"mcp.request":       100 * time.Millisecond,
			"mcp.tools/call":    100 * time.Millisecond,
			"worker.lease":      200 * time.Millisecond,
			"worker.execute":    2 * time.Second,
			"ops.raw_to_minute": 2 * time.Second,
			"ops.retention":     2 * time.Second,
		}
	}
	return cfg
}

func parseLevel(v string) Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(BasicLevel):
		return BasicLevel
	case string(DebugLevel):
		return DebugLevel
	case string(TraceLevel):
		return TraceLevel
	default:
		return OffLevel
	}
}

func parseInt(v string, fallback int) int {
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func parseFloat(v string, fallback float64) float64 {
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

func parseThresholds(v string) map[string]time.Duration {
	out := map[string]time.Duration{}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			continue
		}
		d, err := time.ParseDuration(strings.TrimSpace(pieces[1]))
		if err != nil {
			continue
		}
		out[strings.TrimSpace(pieces[0])] = d
	}
	return out
}

func (c Config) Enabled() bool            { return c.Level != OffLevel }
func (c Config) CaptureSpans() bool       { return c.Level == DebugLevel || c.Level == TraceLevel }
func (c Config) CaptureDiagnostics() bool { return c.Level == TraceLevel }
func (c Config) ShouldSample() bool {
	if c.Level == TraceLevel {
		return true
	}
	if c.Level == OffLevel {
		return false
	}
	if c.DefaultSampleRate >= 1 {
		return true
	}
	if c.DefaultSampleRate <= 0 {
		return false
	}
	return float64(time.Now().UnixNano()%1000)/1000.0 < c.DefaultSampleRate
}
func (c Config) SlowThreshold(key string) time.Duration {
	if d, ok := c.SlowThresholds[key]; ok {
		return d
	}
	if d, ok := c.SlowThresholds["default"]; ok {
		return d
	}
	return 0
}
