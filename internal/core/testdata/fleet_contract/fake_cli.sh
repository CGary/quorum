#!/bin/sh
# Fake CLI fixture for the level 1 fleet contract-test harness (FLEET-007).
# Deterministic, no model/LLM call: it only ever prints canned --help text.
#
# Toggle via FLEET_CONTRACT_FIXTURE_MODE=ok|broken (default: ok).
#   ok     -> --help output advertises --model, --print, --print-timeout.
#   broken -> --help output omits --print, simulating a renamed/removed flag.
mode="${FLEET_CONTRACT_FIXTURE_MODE:-ok}"

case "$1" in
  --help)
    if [ "$mode" = "broken" ]; then
      echo "usage: fake_cli [--model MODEL] [--print-timeout DURATION]"
    else
      echo "usage: fake_cli [--model MODEL] [--print PROMPT] [--print-timeout DURATION]"
    fi
    exit 0
    ;;
  *)
    echo "fake_cli: unsupported probe args: $*" >&2
    exit 2
    ;;
esac
