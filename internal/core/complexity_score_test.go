package core

import "testing"

func TestScoreComplexity(t *testing.T) {
	policy := ComplexityPolicy{
		Calibrated:  false,
		SMaxFiles:   2,
		SMaxSymbols: 3,
		LMaxFiles:   5,
	}

	t.Run("S band within bounds", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a.go", "b.go"},
			Symbols:       []string{"X", "Y", "Z"},
		}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "S" {
			t.Errorf("expected S, got %q", res.Band)
		}
		if res.Inputs.FilesCount != 2 || res.Inputs.SymbolsCount != 3 {
			t.Errorf("unexpected inputs: %+v", res.Inputs)
		}
	})

	t.Run("M band outside S bounds without L signals", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a.go", "b.go", "c.go"},
			Symbols:       []string{"X"},
		}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "M" {
			t.Errorf("expected M, got %q", res.Band)
		}
	})

	t.Run("L band via file count above cut", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a", "b", "c", "d", "e", "f"},
		}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "L" {
			t.Errorf("expected L, got %q", res.Band)
		}
	})

	t.Run("L band via migration signal regardless of counts", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a.go"},
			Migration:     true,
		}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "L" {
			t.Errorf("expected L, got %q", res.Band)
		}
		found := false
		for _, s := range res.Signals {
			if s == "migration_signal" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected migration_signal in signals: %v", res.Signals)
		}
	})

	t.Run("L band via public_api signal", func(t *testing.T) {
		bp := ComplexityBlueprint{PublicAPI: true}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "L" {
			t.Errorf("expected L, got %q", res.Band)
		}
	})

	t.Run("L band via schema_change signal", func(t *testing.T) {
		bp := ComplexityBlueprint{SchemaChange: true}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "L" {
			t.Errorf("expected L, got %q", res.Band)
		}
	})

	t.Run("boundary exactly at s cut points resolves to S", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a.go", "b.go"},
			Symbols:       []string{"X", "Y", "Z"},
		}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "S" {
			t.Errorf("expected S at exact s cut, got %q", res.Band)
		}
	})

	t.Run("boundary exactly at l cut point resolves to M not L", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a", "b", "c", "d", "e"},
		}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "M" {
			t.Errorf("expected M at exact l cut (not exceeded), got %q", res.Band)
		}
	})

	t.Run("one file above s cut point resolves to M", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a.go", "b.go", "c.go"},
		}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "M" {
			t.Errorf("expected M just above s cut, got %q", res.Band)
		}
	})

	t.Run("response never exposes a numeric score field", func(t *testing.T) {
		bp := ComplexityBlueprint{AffectedFiles: []string{"a.go"}}
		res, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "S" && res.Band != "M" && res.Band != "L" {
			t.Errorf("band must be S, M, or L, got %q", res.Band)
		}
	})

	t.Run("malformed policy with zero l_max_files errors without panicking", func(t *testing.T) {
		bp := ComplexityBlueprint{AffectedFiles: []string{"a.go"}}
		_, err := ScoreComplexity(bp, ComplexityPolicy{})
		if err == nil {
			t.Fatal("expected error for zero-value policy, got nil")
		}
	})

	t.Run("malformed policy with negative s cut errors without panicking", func(t *testing.T) {
		bp := ComplexityBlueprint{AffectedFiles: []string{"a.go"}}
		badPolicy := ComplexityPolicy{LMaxFiles: 5, SMaxFiles: -1, SMaxSymbols: 3}
		_, err := ScoreComplexity(bp, badPolicy)
		if err == nil {
			t.Fatal("expected error for negative s_max_files, got nil")
		}
	})

	t.Run("AC-1: excludes noncounted_globs matching files from band file count", func(t *testing.T) {
		pol := policy
		pol.NoncountedGlobs = []string{"*_test.go", "*.md"}
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"main.go", "util.go", "main_test.go", "util_test.go", "README.md"},
			Symbols:       []string{"A", "B"},
		}
		res, err := ScoreComplexity(bp, pol)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "S" {
			t.Errorf("expected S (2 production files <= s_max_files 2), got %q", res.Band)
		}
		if res.Inputs.FilesCount != 2 {
			t.Errorf("expected FilesCount 2, got %d", res.Inputs.FilesCount)
		}
	})

	t.Run("AC-2: absent or empty noncounted_globs preserves prior file count", func(t *testing.T) {
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a.go", "b.go", "c.go"},
		}
		res1, err := ScoreComplexity(bp, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res1.Inputs.FilesCount != 3 || res1.Band != "M" {
			t.Errorf("expected FilesCount 3, Band M for nil globs; got %d, %q", res1.Inputs.FilesCount, res1.Band)
		}

		emptyPol := policy
		emptyPol.NoncountedGlobs = []string{}
		res2, err := ScoreComplexity(bp, emptyPol)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res2.Inputs.FilesCount != 3 || res2.Band != "M" {
			t.Errorf("expected FilesCount 3, Band M for empty globs; got %d, %q", res2.Inputs.FilesCount, res2.Band)
		}
	})

	t.Run("AC-5: anchor case bands as M under noncounted_globs", func(t *testing.T) {
		pol := policy
		pol.NoncountedGlobs = []string{"*_test.go", "**/testdata/**", "**/test/**", "**/tests/**", "*.md", "**/docs/**"}
		bp := ComplexityBlueprint{
			AffectedFiles: []string{
				"internal/core/a.go", "internal/core/b.go", "internal/core/c.go", "internal/core/d.go", "internal/core/e.go",
				"internal/core/a_test.go", "internal/core/b_test.go", "internal/core/c_test.go", "internal/core/d_test.go", "internal/core/e_test.go", "internal/core/f_test.go",
				"docs/architecture.md", "README.md",
			},
			Symbols: make([]string, 27),
		}
		res, err := ScoreComplexity(bp, pol)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Inputs.FilesCount != 5 {
			t.Errorf("expected 5 production files counted, got %d", res.Inputs.FilesCount)
		}
		if res.Band != "M" {
			t.Errorf("expected band M, got %q", res.Band)
		}
	})

	t.Run("AC-6: L-forcing signal forces L regardless of noncounted_globs", func(t *testing.T) {
		pol := policy
		pol.NoncountedGlobs = []string{"*_test.go", "*.md"}
		bp := ComplexityBlueprint{
			AffectedFiles: []string{"a.go", "a_test.go", "README.md"},
			Migration:     true,
		}
		res, err := ScoreComplexity(bp, pol)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Band != "L" {
			t.Errorf("expected L due to migration signal, got %q", res.Band)
		}
	})
}
