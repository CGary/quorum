//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hsme/core/src/bootstrap"
)

const cliVersion = "2.0.0"

var (
	exitUsage   = 1
	exitRuntime = 2
)

func main() {
	// 1. Load defaults from env
	cfg := bootstrap.LoadFromEnv()

	// 2. Register global flags on flag.CommandLine
	var versionFlag bool
	flag.CommandLine.BoolVar(&versionFlag, "version", false, "print hsme-cli version and exit")

	var globalAgentFlags AgentFlags
	RegisterAgentFlags(flag.CommandLine, &globalAgentFlags)
	RegisterDBFlags(flag.CommandLine, &cfg)

	// 3. Set custom usage
	flag.Usage = printTopLevelHelp

	// 4. Parse global flags (stops at first non-flag arg)
	flag.Parse()
	// Re-parse trailing global flags
	ScanTrailingFlags(flag.CommandLine)

	// Check top-level version flag
	if versionFlag {
		if globalAgentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "version",
				Summary:     "hsme-cli " + cliVersion,
				Data:        map[string]any{"version": cliVersion},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Println("hsme-cli " + cliVersion)
		}
		os.Exit(0)
	}

	// 5. Remaining args are the subcommand and its flags
	args := flag.Args()
	if len(args) < 1 {
		if globalAgentFlags.JSON {
			emitErrorAndExit("cli", errMissingRequired, "missing subcommand", "subcommand", "", true, "hsme-cli --help", true)
		}
		printTopLevelHelp()
		os.Exit(exitUsage)
	}

	subcommand := args[0]
	subArgs := args[1:]

	// Re-check for help as subcommand
	if subcommand == "help" {
		runHelp(subArgs)
		os.Exit(0)
	}

	// Dispatch
	switch subcommand {
	case "store":
		runStore(subArgs, cfg)
	case "search-fuzzy":
		runSearchFuzzy(subArgs, cfg)
	case "search-exact":
		runSearchExact(subArgs, cfg)
	case "explore":
		runExplore(subArgs, cfg)
	case "status":
		runStatus(subArgs, cfg)
	case "admin":
		runAdmin(subArgs, cfg)
	case "import-quorum":
		runImportQuorum(subArgs, cfg)
	default:
		if globalAgentFlags.JSON {
			emitErrorAndExit("cli", errInvalidArgument, fmt.Sprintf("unknown subcommand: %s", subcommand), "subcommand", subcommand, false, "hsme-cli --help", true)
		}
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", subcommand)
		printTopLevelHelp()
		os.Exit(exitUsage)
	}
}

// ScanTrailingFlags scans the leftover arguments for flags that the FlagSet knows about.
// This allows flags to appear after positional arguments.
func ScanTrailingFlags(fs *flag.FlagSet) {
	args := fs.Args()
	if len(args) == 0 {
		return
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			name := strings.TrimLeft(arg, "-")
			// Handle name=value
			if strings.Contains(name, "=") {
				parts := strings.SplitN(name, "=", 2)
				name = parts[0]
				if f := fs.Lookup(name); f != nil {
					_ = fs.Set(name, parts[1])
				}
				continue
			}

			if f := fs.Lookup(name); f != nil {
				// Is it a bool flag?
				if isBoolFlag(f) {
					_ = fs.Set(name, "true")
				} else if i+1 < len(args) {
					_ = fs.Set(name, args[i+1])
					i++ // Skip value
				}
			}
		}
	}
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}
