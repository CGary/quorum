package core

import "fmt"

// ComplexityBlueprint captures the subset of blueprint data relevant to
// complexity scoring: affected files, symbols, and boolean signal flags
// that unconditionally force the highest band regardless of file/symbol
// counts.
type ComplexityBlueprint struct {
	AffectedFiles []string `json:"affected_files" yaml:"affected_files"`
	Symbols       []string `json:"symbols" yaml:"symbols"`
	Migration     bool     `json:"migration" yaml:"migration"`
	PublicAPI     bool     `json:"public_api" yaml:"public_api"`
	SchemaChange  bool     `json:"schema_change" yaml:"schema_change"`
}

// ComplexityPolicy holds the S/M/L cut points. It is always supplied by the
// caller (populated from .agents/policies/complexity.yaml at runtime) and is
// never hardcoded in this file.
type ComplexityPolicy struct {
	Calibrated  bool `json:"calibrated" yaml:"calibrated"`
	SMaxFiles   int  `json:"s_max_files" yaml:"s_max_files"`
	SMaxSymbols int  `json:"s_max_symbols" yaml:"s_max_symbols"`
	LMaxFiles   int  `json:"l_max_files" yaml:"l_max_files"`
}

// ComplexityInputs echoes the raw values used to compute the band, for
// auditability.
type ComplexityInputs struct {
	FilesCount   int  `json:"files_count"`
	SymbolsCount int  `json:"symbols_count"`
	Migration    bool `json:"migration"`
	PublicAPI    bool `json:"public_api"`
	SchemaChange bool `json:"schema_change"`
}

// ComplexityResult is the advisory output: a band (S/M/L), the signals that
// triggered it, and the inputs used. It never exposes a bare numeric score.
type ComplexityResult struct {
	Band       string           `json:"band"`
	Signals    []string         `json:"signals"`
	Inputs     ComplexityInputs `json:"inputs"`
	Calibrated bool             `json:"calibrated"`
}

// ScoreComplexity computes an advisory S/M/L complexity band from a
// blueprint and policy cut points. It is a pure function: no file I/O, no
// mutation, no reference to model names, executor levels, or routing
// decisions. The policy is validated defensively so malformed input never
// panics; it returns a descriptive error instead.
func ScoreComplexity(bp ComplexityBlueprint, policy ComplexityPolicy) (ComplexityResult, error) {
	if policy.LMaxFiles <= 0 {
		return ComplexityResult{}, fmt.Errorf("invalid policy: l_max_files must be > 0, got %d", policy.LMaxFiles)
	}
	if policy.SMaxFiles < 0 {
		return ComplexityResult{}, fmt.Errorf("invalid policy: s_max_files must be >= 0, got %d", policy.SMaxFiles)
	}
	if policy.SMaxSymbols < 0 {
		return ComplexityResult{}, fmt.Errorf("invalid policy: s_max_symbols must be >= 0, got %d", policy.SMaxSymbols)
	}

	affected := bp.AffectedFiles
	if affected == nil {
		affected = []string{}
	}
	symbols := bp.Symbols
	if symbols == nil {
		symbols = []string{}
	}

	filesCount := len(affected)
	symbolsCount := len(symbols)

	inputs := ComplexityInputs{
		FilesCount:   filesCount,
		SymbolsCount: symbolsCount,
		Migration:    bp.Migration,
		PublicAPI:    bp.PublicAPI,
		SchemaChange: bp.SchemaChange,
	}

	var signals []string
	if bp.Migration {
		signals = append(signals, "migration_signal")
	}
	if bp.PublicAPI {
		signals = append(signals, "public_api_signal")
	}
	if bp.SchemaChange {
		signals = append(signals, "schema_change_signal")
	}
	if filesCount > policy.LMaxFiles {
		signals = append(signals, fmt.Sprintf("file_count_high: %d", filesCount))
	}

	if bp.Migration || bp.PublicAPI || bp.SchemaChange || filesCount > policy.LMaxFiles {
		return ComplexityResult{
			Band:       "L",
			Signals:    signals,
			Inputs:     inputs,
			Calibrated: policy.Calibrated,
		}, nil
	}

	if filesCount <= policy.SMaxFiles && symbolsCount <= policy.SMaxSymbols {
		signals = append(signals, fmt.Sprintf("within_s_bounds: files<=%d symbols<=%d", policy.SMaxFiles, policy.SMaxSymbols))
		return ComplexityResult{
			Band:       "S",
			Signals:    signals,
			Inputs:     inputs,
			Calibrated: policy.Calibrated,
		}, nil
	}

	signals = append(signals, "outside_s_bounds")
	return ComplexityResult{
		Band:       "M",
		Signals:    signals,
		Inputs:     inputs,
		Calibrated: policy.Calibrated,
	}, nil
}
