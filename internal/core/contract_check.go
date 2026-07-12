package core

import "fmt"

// ContractLimits mirrors the optional limits block of 02-contract.yaml.
// Pointers distinguish "field absent" from "field set to zero" so rules 3
// and 4 can be skipped without error when the contract does not define
// limits at all (00-spec.yaml invariant, AC-6).
type ContractLimits struct {
	MaxFilesChanged *int `json:"max_files_changed,omitempty" yaml:"max_files_changed"`
	MaxDiffLines    *int `json:"max_diff_lines,omitempty" yaml:"max_diff_lines"`
}

// ContractForbid mirrors the forbid block of 02-contract.yaml.
type ContractForbid struct {
	Files     []string `json:"files" yaml:"files"`
	Behaviors []string `json:"behaviors" yaml:"behaviors"`
}

// Contract is the subset of 02-contract.yaml relevant to CheckContract.
type Contract struct {
	Touch  []string        `json:"touch" yaml:"touch"`
	Forbid ContractForbid  `json:"forbid" yaml:"forbid"`
	Limits *ContractLimits `json:"limits,omitempty" yaml:"limits"`
}

// DiffStat carries caller-computed line counts for the changed files. It is
// always supplied by the caller; CheckContract never computes it.
type DiffStat struct {
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

// ContractViolation is an actionable report of a single contract breach: the
// offending file (when applicable), the violated rule, and what the
// contract expected.
type ContractViolation struct {
	Type   string `json:"type"`
	File   string `json:"file,omitempty"`
	Rule   string `json:"rule"`
	Detail string `json:"detail"`
}

// ContractCheckResult is the deterministic output of CheckContract.
type ContractCheckResult struct {
	Ok         bool                `json:"ok"`
	Violations []ContractViolation `json:"violations"`
	NotChecked []string            `json:"not_checked"`
}

// CheckContract is a pure function: it never executes git, never touches
// the filesystem, and never computes changed_files or diff_stat itself --
// those are always supplied by the caller. It evaluates a contract's
// touch/forbid.files/limits rules deterministically and reports violations;
// it never decides whether a violation blocks the task (that remains the
// caller's decision). forbid.behaviors is a semantic judgment left to
// q-review, so it is always surfaced via not_checked, never silently
// evaluated or omitted.
func CheckContract(contract Contract, changedFiles []string, diffStat DiffStat) ContractCheckResult {
	violations := []ContractViolation{}

	// Rule 1: every changed file (including newly created files) must match
	// at least one touch glob, or it is a touch violation.
	for _, f := range changedFiles {
		matched := false
		for _, g := range contract.Touch {
			if safeGlobMatch(f, g) {
				matched = true
				break
			}
		}
		if !matched {
			violations = append(violations, ContractViolation{
				Type:   "touch",
				File:   f,
				Rule:   "touch",
				Detail: fmt.Sprintf("file %q is not covered by any touch glob in the contract", f),
			})
		}
	}

	// Rule 2: a changed file matching forbid.files is always a violation,
	// even if it also matches touch.
	for _, f := range changedFiles {
		for _, g := range contract.Forbid.Files {
			if safeGlobMatch(f, g) {
				violations = append(violations, ContractViolation{
					Type:   "forbid_files",
					File:   f,
					Rule:   "forbid.files",
					Detail: fmt.Sprintf("file %q matches forbidden pattern %q", f, g),
				})
				break
			}
		}
	}

	// Rules 3 and 4: skipped without error when the contract has no limits
	// block at all.
	if contract.Limits != nil {
		if contract.Limits.MaxDiffLines != nil {
			total := diffStat.Insertions + diffStat.Deletions
			if total > *contract.Limits.MaxDiffLines {
				violations = append(violations, ContractViolation{
					Type:   "max_diff_lines",
					Rule:   "limits.max_diff_lines",
					Detail: fmt.Sprintf("diff of %d lines exceeds limits.max_diff_lines=%d", total, *contract.Limits.MaxDiffLines),
				})
			}
		}
		if contract.Limits.MaxFilesChanged != nil {
			if len(changedFiles) > *contract.Limits.MaxFilesChanged {
				violations = append(violations, ContractViolation{
					Type:   "max_files_changed",
					Rule:   "limits.max_files_changed",
					Detail: fmt.Sprintf("%d changed files exceeds limits.max_files_changed=%d", len(changedFiles), *contract.Limits.MaxFilesChanged),
				})
			}
		}
	}

	return ContractCheckResult{
		Ok:         len(violations) == 0,
		Violations: violations,
		// forbid.behaviors is a semantic judgment (q-review's domain, not
		// this pure checker's), so it is always reported as not_checked --
		// regardless of the outcome of touch/forbid.files/limits -- so no
		// consumer assumes full coverage.
		NotChecked: []string{"forbid.behaviors"},
	}
}
