package core

import "fmt"

// Agy adapter guard (FLEET-007): agy has no engine-level wiring changes of
// its own -- the generic FLEET-006-a dispatch engine and agents.yaml already
// describe how to invoke it. This file adds only the defensive argv-order
// guard called out by Fase 0a Sec8.1 and a small usage-reporting helper; it
// never runs the agy process itself.

// ValidateAgyArgv guards the Fase 0a Sec8.1 greedy-flag trap: agy's --print/-p
// flag is a greedy Go-style string flag that consumes the very next argv
// token as the prompt. If any other flag were placed after --print/-p, THAT
// flag would be silently swallowed as the prompt instead of erroring -- a
// false-success trap, not a crash. This is a pure, process-free check over
// the argv slice the wrapper is about to exec; it never invokes agy.
func ValidateAgyArgv(argv []string) error {
	printIdx := -1
	for i, tok := range argv {
		if tok == "--print" || tok == "-p" {
			printIdx = i
			break
		}
	}
	if printIdx == -1 {
		return fmt.Errorf("agy argv guard: no --print/-p flag found in argv %v", argv)
	}
	// The prompt must be the very last token, and --print/-p must be the token
	// immediately before it -- nothing may follow the prompt, and no flag may
	// sit between --print/-p and the prompt.
	if printIdx != len(argv)-2 {
		return fmt.Errorf("agy argv guard: --print/-p must sit second-to-last with the prompt as the final token; got --print/-p at index %d of %d tokens: %v", printIdx, len(argv), argv)
	}
	return nil
}

// AgyUsage returns the DispatchUsage for an agy dispatch. agy has no
// usage/token reporting (Fase 0a Sec3), so Source is always "none" -- an agy
// dispatch must never report Source "cli_reported", and never fabricates
// TokensIn/TokensOut/Requests.
func AgyUsage() DispatchUsage {
	return DispatchUsage{Source: "none"}
}
