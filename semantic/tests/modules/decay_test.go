package modules

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/hsme/core/src/core/search"
)

func TestAgeInDays(t *testing.T) {
	now := time.Now()

	// 1 day ago
	createdAt := now.Add(-24 * time.Hour)
	age := search.AgeInDays(now, createdAt)
	if math.Abs(age-1.0) > 0.001 {
		t.Errorf("expected age approx 1.0, got %f", age)
	}

	// 7 days ago
	createdAt = now.Add(-7 * 24 * time.Hour)
	age = search.AgeInDays(now, createdAt)
	if math.Abs(age-7.0) > 0.001 {
		t.Errorf("expected age approx 7.0, got %f", age)
	}

	// Future timestamp (should clamp to 0)
	createdAt = now.Add(24 * time.Hour)
	age = search.AgeInDays(now, createdAt)
	if age != 0 {
		t.Errorf("expected future age to clamp to 0, got %f", age)
	}

	// Exactly now
	age = search.AgeInDays(now, now)
	if age != 0 {
		t.Errorf("expected age 0, got %f", age)
	}
}

func TestDecayFactor(t *testing.T) {
	halfLife := 14.0

	// Age 0 -> factor 1.0
	factor := search.DecayFactor(0, halfLife)
	if factor != 1.0 {
		t.Errorf("expected factor 1.0 for age 0, got %f", factor)
	}

	// Age = half-life -> factor 0.5
	factor = search.DecayFactor(halfLife, halfLife)
	if factor != 0.5 {
		t.Errorf("expected factor 0.5 for age = half-life, got %f", factor)
	}

	// Age = 2 * half-life -> factor 0.25
	factor = search.DecayFactor(halfLife*2, halfLife)
	if factor != 0.25 {
		t.Errorf("expected factor 0.25 for age = 2 * half-life, got %f", factor)
	}

	// Negative age -> factor 1.0
	factor = search.DecayFactor(-5, halfLife)
	if factor != 1.0 {
		t.Errorf("expected factor 1.0 for negative age, got %f", factor)
	}

	// Invalid half-life (<= 0) -> factor 1.0
	factor = search.DecayFactor(10, 0)
	if factor != 1.0 {
		t.Errorf("expected failsafe factor 1.0 for zero half-life, got %f", factor)
	}
}

func TestLoadDecayConfig(t *testing.T) {
	// Helper to clear env
	clearEnv := func() {
		os.Unsetenv("RRF_TIME_DECAY")
		os.Unsetenv("RRF_HALF_LIFE_DAYS")
	}

	defer clearEnv()

	// Default config
	clearEnv()
	cfg, err := search.LoadDecayConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Enabled {
		t.Errorf("expected default config to be disabled")
	}
	if cfg.HalfLifeDays != 14.0 {
		t.Errorf("expected default half-life 14.0, got %f", cfg.HalfLifeDays)
	}

	// Valid explicitly disabled
	clearEnv()
	os.Setenv("RRF_TIME_DECAY", "off")
	cfg, err = search.LoadDecayConfig()
	if err != nil || cfg.Enabled {
		t.Errorf("expected off to be disabled and valid")
	}

	// Valid enabled
	clearEnv()
	os.Setenv("RRF_TIME_DECAY", "on")
	os.Setenv("RRF_HALF_LIFE_DAYS", "7.5")
	cfg, err = search.LoadDecayConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Enabled {
		t.Errorf("expected config to be enabled")
	}
	if cfg.HalfLifeDays != 7.5 {
		t.Errorf("expected half-life 7.5, got %f", cfg.HalfLifeDays)
	}

	// Invalid RRF_TIME_DECAY
	clearEnv()
	os.Setenv("RRF_TIME_DECAY", "invalid")
	_, err = search.LoadDecayConfig()
	if err == nil {
		t.Errorf("expected error for invalid RRF_TIME_DECAY")
	}

	// Strict flag contract: only on/off are accepted.
	clearEnv()
	os.Setenv("RRF_TIME_DECAY", "true")
	_, err = search.LoadDecayConfig()
	if err == nil {
		t.Errorf("expected error for true; only on/off are accepted")
	}

	// Invalid RRF_HALF_LIFE_DAYS (not a number)
	clearEnv()
	os.Setenv("RRF_TIME_DECAY", "on")
	os.Setenv("RRF_HALF_LIFE_DAYS", "not_a_number")
	_, err = search.LoadDecayConfig()
	if err == nil {
		t.Errorf("expected error for non-numeric half-life")
	}

	// Invalid RRF_HALF_LIFE_DAYS (<= 0)
	clearEnv()
	os.Setenv("RRF_HALF_LIFE_DAYS", "0")
	_, err = search.LoadDecayConfig()
	if err == nil {
		t.Errorf("expected error for zero half-life")
	}
}
