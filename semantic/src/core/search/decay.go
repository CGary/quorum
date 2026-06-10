package search

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// DecayConfig holds the configuration for time-based result decay.
type DecayConfig struct {
	Enabled      bool
	HalfLifeDays float64
}

// Global configuration for decay, loaded at startup.
var GlobalDecayConfig DecayConfig

// LoadDecayConfig parses the RRF_TIME_DECAY and RRF_HALF_LIFE_DAYS environment variables.
func LoadDecayConfig() (DecayConfig, error) {
	config := DecayConfig{
		Enabled:      false,
		HalfLifeDays: 14.0, // default half-life
	}

	decayEnv := strings.TrimSpace(os.Getenv("RRF_TIME_DECAY"))
	if decayEnv == "on" {
		config.Enabled = true
	} else if decayEnv != "" && decayEnv != "off" {
		return config, fmt.Errorf("invalid value for RRF_TIME_DECAY: %q (expected 'on' or 'off')", decayEnv)
	}

	halfLifeEnv := strings.TrimSpace(os.Getenv("RRF_HALF_LIFE_DAYS"))
	if halfLifeEnv != "" {
		hl, err := strconv.ParseFloat(halfLifeEnv, 64)
		if err != nil {
			return config, fmt.Errorf("invalid value for RRF_HALF_LIFE_DAYS: %q (must be a number)", halfLifeEnv)
		}
		if hl <= 0 {
			return config, fmt.Errorf("invalid value for RRF_HALF_LIFE_DAYS: %f (must be strictly positive)", hl)
		}
		config.HalfLifeDays = hl
	}

	return config, nil
}

// AgeInDays calculates the age in days between a reference time (usually now) and a creation time.
// It clamps negative ages (future timestamps) to 0.
func AgeInDays(reference time.Time, createdAt time.Time) float64 {
	age := reference.Sub(createdAt).Hours() / 24.0
	if age < 0 {
		return 0
	}
	return age
}

// DecayFactor computes the exponential decay factor given the age in days and the half-life in days.
// The formula is: 0.5 ^ (max(0, ageDays) / halfLifeDays).
func DecayFactor(ageDays float64, halfLifeDays float64) float64 {
	if ageDays < 0 {
		ageDays = 0
	}
	if halfLifeDays <= 0 {
		return 1.0 // Failsafe, though validation should prevent this
	}
	return math.Pow(0.5, ageDays/halfLifeDays)
}
