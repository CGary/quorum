# Contract: `DecayFactor` pure function

**Mission**: `universal-time-decay-for-search-results-01KQ4631`

The decay function is the single mathematical primitive that drives this mission. It is pure, has no I/O, and is the only place the half-life math lives.

---

## Signature

```go
package search

// DecayFactor returns a multiplier in (0, 1] given a memory's age in days
// and the configured half-life. Negative ages clamp to 0, yielding 1.0.
//
// Caller is responsible for ensuring halfLifeDays > 0; this function does NOT
// validate. Validation belongs to LoadDecayConfig.
func DecayFactor(ageDays float64, halfLifeDays float64) float64
```

## Implementation

```go
import "math"

func DecayFactor(ageDays float64, halfLifeDays float64) float64 {
    if ageDays < 0 {
        ageDays = 0
    }
    return math.Pow(0.5, ageDays/halfLifeDays)
}
```

## Properties

| Property | Statement |
|----------|-----------|
| **Bounded** | `0 < DecayFactor(a, h) <= 1` for all `a >= 0` and `h > 0` |
| **Monotonic** | `a1 < a2` implies `DecayFactor(a1, h) >= DecayFactor(a2, h)` (older memories never get a higher factor) |
| **Half-life** | `DecayFactor(h, h) == 0.5` |
| **Zero-age** | `DecayFactor(0, h) == 1.0` |
| **Future-clamped** | `DecayFactor(-x, h) == 1.0` for any `x > 0`, any `h > 0` |
| **Pure** | No global state read, no I/O, no panics, no error return |

## Test cases (unit tests in `tests/modules/decay_test.go`)

```go
TestDecayFactor_Boundaries:
    DecayFactor(0,   14) == 1.0
    DecayFactor(14,  14) == 0.5
    DecayFactor(28,  14) == 0.25
    DecayFactor(56,  14) == 0.0625

TestDecayFactor_FutureClamp:
    DecayFactor(-5,  14) == 1.0
    DecayFactor(-1e9, 14) == 1.0

TestDecayFactor_HalfLifeShape:
    For h ∈ {1, 7, 14, 30, 90}:
        DecayFactor(h, h) ≈ 0.5  (within 1e-12)

TestDecayFactor_Bounded:
    For 100 random (age, h) pairs with age >= 0 and h > 0:
        0 < DecayFactor(age, h) <= 1
```

## Helper: `ageInDays`

```go
package search

import "time"

// ageInDays returns the age of a memory at the given query time, in days,
// computed against the memory's parsed created_at. Negative results are
// returned as-is; the caller (DecayFactor) clamps them.
func ageInDays(now time.Time, createdAt time.Time) float64
```

Implementation: `now.Sub(createdAt).Hours() / 24`. Always defined for valid times.

The `created_at` column in `memories` may be stored in a few formats (the corpus has both `2026-04-25 23:47:11.830717127-04:00` and `2026-04-24 04:04:23` shapes per the actual data). `ageInDays`'s caller is responsible for parsing strings; the helper itself takes already-parsed `time.Time` values.

A separate helper `parseCreatedAt(s string) (time.Time, error)` lives in `decay.go` and tries multiple layouts in order:
1. `time.RFC3339Nano`
2. `"2006-01-02 15:04:05.999999999-07:00"`
3. `"2006-01-02 15:04:05"` (UTC assumed)

If all parses fail, `parseCreatedAt` returns an error AND the calling search function MUST treat that memory as `decay = 1.0` (defensive — better than dropping or scoring incorrectly). A log message at debug level records the parse failure.
