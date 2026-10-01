package core

import (
	"strings"
	"testing"
)

// validJevPolicy returns a policy that passes every validation rule. Tests
// mutate one field at a time to exercise a single rule.
func validJevPolicy() JevRouterPolicy {
	return JevRouterPolicy{
		Version:  1,
		Provider: "typesafe",
		Providers: map[string]JevProviderConfig{
			"typesafe": {URL: "https://jev.example/decide", Model: "classifier-v1", APIKeyEnv: "JEV_API_KEY"},
		},
		TimeoutS:               30,
		MaxStateTokens:         1000,
		Transport:              "opencode_go",
		Instructions:           "Pick the best model.",
		DifficultyInstructions: "How hard is this task?",
		Candidates: []JevCandidate{
			{Key: "a", Model: "vendor/model-a", What: "cheap", NotFor: "hard tasks", Examples: []string{"small edit"}, MaxDifficulty: "hard"},
			{Key: "b", Model: "vendor/model-b", What: "mid", Evidence: []string{"human-only note"}, MaxDifficulty: "hard"},
			{Key: "c", Model: "vendor/model-c", What: "strong", MaxDifficulty: "hard"},
		},
		Difficulty: []JevDifficultyLevel{
			{Key: "easy", Description: "easy task", ClaudeModel: "haiku"},
			{Key: "hard", Description: "hard task", ClaudeModel: "opus"},
		},
		DefaultDifficulty: "easy",
		FallbackPair:      []string{"a", "b"},
	}
}

func TestValidateJevRouterPolicyValid(t *testing.T) {
	if err := ValidateJevRouterPolicy(validJevPolicy()); err != nil {
		t.Fatalf("valid policy must pass: %v", err)
	}
}

func TestValidateJevRouterPolicyRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*JevRouterPolicy)
		wantSub string
	}{
		{"provider_not_known", func(p *JevRouterPolicy) { p.Provider = "mystery" }, "provider"},
		{"provider_missing_from_map", func(p *JevRouterPolicy) {
			p.Provider = "openrouter_decisions"
		}, "providers"},
		{"provider_url_empty", func(p *JevRouterPolicy) {
			cfg := p.Providers["typesafe"]
			cfg.URL = ""
			p.Providers["typesafe"] = cfg
		}, "url"},
		{"provider_model_empty", func(p *JevRouterPolicy) {
			cfg := p.Providers["typesafe"]
			cfg.Model = ""
			p.Providers["typesafe"] = cfg
		}, "model"},
		{"provider_api_key_env_empty", func(p *JevRouterPolicy) {
			cfg := p.Providers["typesafe"]
			cfg.APIKeyEnv = ""
			p.Providers["typesafe"] = cfg
		}, "api_key_env"},
		{"timeout_nonpositive", func(p *JevRouterPolicy) { p.TimeoutS = 0 }, "timeout_s"},
		{"max_state_tokens_nonpositive", func(p *JevRouterPolicy) { p.MaxStateTokens = -1 }, "max_state_tokens"},
		{"transport_empty", func(p *JevRouterPolicy) { p.Transport = "" }, "transport"},
		{"instructions_empty", func(p *JevRouterPolicy) { p.Instructions = "" }, "instructions"},
		{"difficulty_instructions_empty", func(p *JevRouterPolicy) { p.DifficultyInstructions = "" }, "difficulty_instructions"},
		{"candidates_empty", func(p *JevRouterPolicy) { p.Candidates = nil }, "candidates"},
		{"candidates_too_many", func(p *JevRouterPolicy) {
			p.Candidates = make([]JevCandidate, 256)
			for i := range p.Candidates {
				p.Candidates[i] = JevCandidate{Key: "k", Model: "vendor/m", What: "w"}
			}
		}, "255"},
		{"candidate_key_empty", func(p *JevRouterPolicy) { p.Candidates[1].Key = "" }, "key"},
		{"candidate_key_duplicate", func(p *JevRouterPolicy) { p.Candidates[2].Key = "a" }, "duplicate"},
		{"candidate_model_empty", func(p *JevRouterPolicy) { p.Candidates[1].Model = "" }, "model"},
		{"candidate_model_duplicate", func(p *JevRouterPolicy) { p.Candidates[1].Model = "vendor/model-a" }, "duplicate model"},
		{"candidate_what_empty", func(p *JevRouterPolicy) { p.Candidates[1].What = "" }, "what"},
		{"candidate_cost_tier_invalid", func(p *JevRouterPolicy) { p.Candidates[0].CostTier = "ultra" }, "cost_tier"},
		{"candidate_max_difficulty_missing", func(p *JevRouterPolicy) { p.Candidates[1].MaxDifficulty = "" }, "max_difficulty"},
		{"candidate_max_difficulty_unknown", func(p *JevRouterPolicy) { p.Candidates[0].MaxDifficulty = "medium" }, "max_difficulty"},
		{"difficulty_too_few", func(p *JevRouterPolicy) { p.Difficulty = p.Difficulty[:1] }, "difficulty"},
		{"difficulty_key_empty", func(p *JevRouterPolicy) { p.Difficulty[0].Key = "" }, "difficulty[0].key"},
		{"difficulty_key_duplicate", func(p *JevRouterPolicy) { p.Difficulty[1].Key = "easy" }, "duplicate"},
		{"difficulty_claude_model_invalid", func(p *JevRouterPolicy) { p.Difficulty[0].ClaudeModel = "mega" }, "claude_model"},
		{"default_difficulty_unknown", func(p *JevRouterPolicy) { p.DefaultDifficulty = "medium" }, "default_difficulty"},
		{"fallback_pair_wrong_len", func(p *JevRouterPolicy) { p.FallbackPair = []string{"a"} }, "fallback_pair"},
		{"fallback_pair_duplicate", func(p *JevRouterPolicy) { p.FallbackPair = []string{"a", "a"} }, "distinct"},
		{"fallback_pair_unknown_key", func(p *JevRouterPolicy) { p.FallbackPair = []string{"a", "zz"} }, "fallback_pair[1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validJevPolicy()
			tc.mutate(&p)
			err := ValidateJevRouterPolicy(p)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestValidateJevRouterPolicyCostTierValid(t *testing.T) {
	p := validJevPolicy()
	p.Candidates[0].CostTier = "low"
	p.Candidates[1].CostTier = "medium"
	p.Candidates[2].CostTier = "high"
	if err := ValidateJevRouterPolicy(p); err != nil {
		t.Fatalf("low/medium/high cost tiers must pass: %v", err)
	}
}

// validJevPolicyWithBackup returns a valid policy whose first candidate carries
// a paired backup, so tests can mutate one backup field at a time.
func validJevPolicyWithBackup() JevRouterPolicy {
	p := validJevPolicy()
	p.Candidates[0].Backup = &JevBackup{Key: "a-backup", Model: "vendor/model-a2", Evidence: []string{"note"}}
	return p
}

func TestValidateJevRouterPolicyBackupRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*JevRouterPolicy)
		wantSub string
	}{
		{"backup_key_empty", func(p *JevRouterPolicy) { p.Candidates[0].Backup.Key = "" }, "backup.key"},
		{"backup_model_empty", func(p *JevRouterPolicy) { p.Candidates[0].Backup.Model = "" }, "backup.model"},
		{"backup_model_same_as_candidate", func(p *JevRouterPolicy) { p.Candidates[0].Backup.Model = p.Candidates[0].Model }, "differ"},
		{"backup_key_duplicate", func(p *JevRouterPolicy) {
			p.Candidates[1].Backup = &JevBackup{Key: "a-backup", Model: "vendor/model-b2"}
		}, "duplicate backup key"},
		{"backup_model_duplicate", func(p *JevRouterPolicy) {
			p.Candidates[1].Backup = &JevBackup{Key: "b-backup", Model: "vendor/model-a2"}
		}, "duplicate backup model"},
		{"backup_key_collides_candidate", func(p *JevRouterPolicy) { p.Candidates[0].Backup.Key = "b" }, "collides with candidate key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validJevPolicyWithBackup()
			tc.mutate(&p)
			err := ValidateJevRouterPolicy(p)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestFilterLiveCandidates(t *testing.T) {
	p := validJevPolicy()
	allModels := map[string]bool{
		"vendor/model-a": true,
		"vendor/model-b": true,
		"vendor/model-c": true,
	}

	t.Run("all_live_order_kept", func(t *testing.T) {
		live, dropped := FilterLiveCandidates(p, true, allModels, ControlState{})
		if len(live) != 3 || len(dropped) != 0 {
			t.Fatalf("got %d live %d dropped, want 3/0", len(live), len(dropped))
		}
		for i, want := range []string{"a", "b", "c"} {
			if live[i].Key != want {
				t.Fatalf("live[%d]=%q want %q", i, live[i].Key, want)
			}
		}
	})

	t.Run("transport_inactive", func(t *testing.T) {
		live, dropped := FilterLiveCandidates(p, false, allModels, ControlState{})
		if len(live) != 0 || len(dropped) != 3 {
			t.Fatalf("got %d live %d dropped, want 0/3", len(live), len(dropped))
		}
		for _, d := range dropped {
			if d.Reason != "transport_inactive" {
				t.Fatalf("reason %q want transport_inactive", d.Reason)
			}
		}
	})

	t.Run("not_in_catalog", func(t *testing.T) {
		models := map[string]bool{"vendor/model-a": true, "vendor/model-c": true}
		live, dropped := FilterLiveCandidates(p, true, models, ControlState{})
		if len(live) != 2 || len(dropped) != 1 {
			t.Fatalf("got %d live %d dropped, want 2/1", len(live), len(dropped))
		}
		if dropped[0].Key != "b" || dropped[0].Reason != "not_in_catalog" {
			t.Fatalf("dropped %+v want key b reason not_in_catalog", dropped[0])
		}
		if live[0].Key != "a" || live[1].Key != "c" {
			t.Fatalf("order broken: %q %q", live[0].Key, live[1].Key)
		}
	})

	t.Run("kill_switch_transport", func(t *testing.T) {
		control := ControlState{Disabled: []ControlEntry{{Target: "opencode_go"}}}
		live, dropped := FilterLiveCandidates(p, true, allModels, control)
		if len(live) != 0 || len(dropped) != 3 {
			t.Fatalf("got %d live %d dropped, want 0/3", len(live), len(dropped))
		}
		for _, d := range dropped {
			if d.Reason != "kill_switch" {
				t.Fatalf("reason %q want kill_switch", d.Reason)
			}
		}
	})

	t.Run("kill_switch_model", func(t *testing.T) {
		control := ControlState{Disabled: []ControlEntry{{Target: "opencode_go/vendor/model-b"}}}
		live, dropped := FilterLiveCandidates(p, true, allModels, control)
		if len(live) != 2 || len(dropped) != 1 {
			t.Fatalf("got %d live %d dropped, want 2/1", len(live), len(dropped))
		}
		if dropped[0].Key != "b" || dropped[0].Reason != "kill_switch" {
			t.Fatalf("dropped %+v want key b reason kill_switch", dropped[0])
		}
	})
}

func TestFilterLiveCandidatesBackup(t *testing.T) {
	p := validJevPolicy()
	p.Candidates[0].Backup = &JevBackup{Key: "a-backup", Model: "vendor/model-a2"}
	allModels := map[string]bool{
		"vendor/model-a":  true,
		"vendor/model-b":  true,
		"vendor/model-c":  true,
		"vendor/model-a2": true,
	}

	t.Run("backup_kept_when_live", func(t *testing.T) {
		live, dropped := FilterLiveCandidates(p, true, allModels, ControlState{})
		if len(live) != 3 || len(dropped) != 0 {
			t.Fatalf("got %d/%d want 3/0", len(live), len(dropped))
		}
		if live[0].Backup == nil || live[0].Backup.Key != "a-backup" {
			t.Fatalf("backup must survive, got %+v", live[0].Backup)
		}
	})

	t.Run("backup_not_in_catalog", func(t *testing.T) {
		models := map[string]bool{
			"vendor/model-a": true,
			"vendor/model-b": true,
			"vendor/model-c": true,
		}
		live, dropped := FilterLiveCandidates(p, true, models, ControlState{})
		if live[0].Backup != nil {
			t.Fatalf("backup must be nil'd, got %+v", live[0].Backup)
		}
		if len(dropped) != 1 || dropped[0].Key != "a-backup" || dropped[0].Model != "vendor/model-a2" || dropped[0].Reason != "backup_not_in_catalog" {
			t.Fatalf("dropped %+v want a-backup backup_not_in_catalog", dropped)
		}
		if p.Candidates[0].Backup == nil {
			t.Fatal("policy must not be mutated")
		}
	})

	t.Run("backup_kill_switch", func(t *testing.T) {
		control := ControlState{Disabled: []ControlEntry{{Target: "opencode_go/vendor/model-a2"}}}
		live, dropped := FilterLiveCandidates(p, true, allModels, control)
		if live[0].Backup != nil {
			t.Fatalf("backup must be nil'd, got %+v", live[0].Backup)
		}
		if len(dropped) != 1 || dropped[0].Key != "a-backup" || dropped[0].Reason != "backup_kill_switch" {
			t.Fatalf("dropped %+v want a-backup backup_kill_switch", dropped)
		}
		if p.Candidates[0].Backup == nil {
			t.Fatal("policy must not be mutated")
		}
	})
}

func TestBuildJevRequestShape(t *testing.T) {
	p := validJevPolicy()
	req, trimmed, _, err := BuildJevRequest(p, "typesafe", p.Candidates, "the prompt", "the context")
	if err != nil {
		t.Fatal(err)
	}
	if trimmed {
		t.Fatal("unexpected trim on small request")
	}
	if req.Model != "classifier-v1" {
		t.Fatalf("model %q want classifier-v1", req.Model)
	}
	if req.State["prompt"] != "the prompt" {
		t.Fatalf("prompt %q", req.State["prompt"])
	}
	if req.State["context"] != "the context" {
		t.Fatalf("context %q", req.State["context"])
	}
	tm, ok := req.Questions["target_model"]
	if !ok {
		t.Fatal("missing target_model question")
	}
	if tm.Type != "choice" || tm.Instructions != p.Instructions {
		t.Fatalf("target_model question %+v", tm)
	}
	df, ok := req.Questions["difficulty"]
	if !ok {
		t.Fatal("missing difficulty question")
	}
	if df.Type != "choice" || df.Instructions != p.DifficultyInstructions {
		t.Fatalf("difficulty question %+v", df)
	}

	critA, ok := tm.Criteria["a"].(map[string]any)
	if !ok {
		t.Fatalf("criteria[a] %T", tm.Criteria["a"])
	}
	if critA["what"] != "cheap" || critA["not_for"] != "hard tasks" {
		t.Fatalf("criteria[a] %+v", critA)
	}
	if _, has := critA["examples"]; !has {
		t.Fatal("criteria[a] missing examples")
	}
	critB, ok := tm.Criteria["b"].(map[string]any)
	if !ok {
		t.Fatalf("criteria[b] %T", tm.Criteria["b"])
	}
	if _, has := critB["not_for"]; has {
		t.Fatal("criteria[b] must omit empty not_for")
	}
	if _, has := critB["examples"]; has {
		t.Fatal("criteria[b] must omit empty examples")
	}
	if _, has := critB["evidence"]; has {
		t.Fatal("criteria[b] must never include evidence")
	}

	if df.Criteria["easy"] != "easy task" {
		t.Fatalf("difficulty criteria easy %v", df.Criteria["easy"])
	}
}

func TestBuildJevRequestOmitsEmptyContext(t *testing.T) {
	p := validJevPolicy()
	req, _, _, err := BuildJevRequest(p, "typesafe", p.Candidates, "prompt", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, has := req.State["context"]; has {
		t.Fatal("empty context must be omitted")
	}
}

func TestBuildJevRequestCostTierAndNoBackup(t *testing.T) {
	p := validJevPolicy()
	p.Candidates[0].CostTier = "low"
	p.Candidates[1].CostTier = "high"
	p.Candidates[0].Backup = &JevBackup{Key: "a-backup", Model: "vendor/model-a2"}
	req, _, _, err := BuildJevRequest(p, "typesafe", p.Candidates, "prompt", "")
	if err != nil {
		t.Fatal(err)
	}
	tm := req.Questions["target_model"]
	critA := tm.Criteria["a"].(map[string]any)
	if critA["cost"] != "low" {
		t.Fatalf("criteria[a].cost %v want low", critA["cost"])
	}
	critB := tm.Criteria["b"].(map[string]any)
	if critB["cost"] != "high" {
		t.Fatalf("criteria[b].cost %v want high", critB["cost"])
	}
	critC := tm.Criteria["c"].(map[string]any)
	if _, has := critC["cost"]; has {
		t.Fatal("candidate c has no cost_tier, criteria must omit cost")
	}
	if _, has := tm.Criteria["a-backup"]; has {
		t.Fatal("backup key must never appear as a criteria key")
	}
}

func TestBuildJevRequestTrimsContextTail(t *testing.T) {
	p := validJevPolicy()
	base, _, _, err := BuildJevRequest(p, "typesafe", p.Candidates, "short prompt", "")
	if err != nil {
		t.Fatal(err)
	}
	// Budget = request without context + room for a short context tail.
	p.MaxStateTokens = EstimateJevTokens(base) + 60
	longContext := strings.Repeat("contextual detail. ", 200)
	req, trimmed, est, err := BuildJevRequest(p, "typesafe", p.Candidates, "short prompt", longContext)
	if err != nil {
		t.Fatal(err)
	}
	if !trimmed {
		t.Fatal("expected trimming for oversized request")
	}
	ctx, has := req.State["context"]
	if !has {
		t.Fatal("context was dropped entirely but a tail should fit")
	}
	if !strings.HasSuffix(longContext, ctx) {
		t.Fatal("trimmed context is not a tail of the original")
	}
	if est > p.MaxStateTokens {
		t.Fatalf("estimated tokens %d still over budget %d", est, p.MaxStateTokens)
	}
}

func TestBuildJevRequestPromptTooLarge(t *testing.T) {
	p := validJevPolicy()
	p.MaxStateTokens = 10
	_, trimmed, _, err := BuildJevRequest(p, "typesafe", p.Candidates, strings.Repeat("x", 500), "context")
	if err != ErrJevStateTooLarge {
		t.Fatalf("err %v want ErrJevStateTooLarge", err)
	}
	if !trimmed {
		t.Fatal("expected trimmed=true on over-budget prompt")
	}
}

func choiceAnswer(choice string, probs map[string]float64) JevChoiceAnswer {
	return JevChoiceAnswer{Type: "choice", Choice: choice, Probabilities: probs}
}

func TestDecidePickNormalRanking(t *testing.T) {
	p := validJevPolicy()
	live := p.Candidates
	usage := JevUsage{InputTokens: 10, OutputTokens: 3}
	resp := &JevResponse{
		Model:    "classifier-v1",
		ID:       "resp-1",
		Provider: "typesafe",
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("b", map[string]float64{"a": 0.2, "b": 0.7, "c": 0.1}),
			"difficulty":   choiceAnswer("easy", nil),
		},
		Usage: usage,
	}
	dec := DecidePick(p, live, resp, nil)
	if dec.Reason != "jev" || dec.JevChoice != "b" {
		t.Fatalf("reason/choice %q/%q want jev/b", dec.Reason, dec.JevChoice)
	}
	if dec.Primary.Key != "b" || dec.Primary.Model != "vendor/model-b" {
		t.Fatalf("primary %+v want b", dec.Primary)
	}
	if dec.Backup == nil || dec.Backup.Key != "a" {
		t.Fatalf("backup %+v want a", dec.Backup)
	}
	if dec.Primary.Probability == nil || *dec.Primary.Probability != 0.7 {
		t.Fatalf("primary probability %v want 0.7", dec.Primary.Probability)
	}
	if dec.JevModel != "classifier-v1" || dec.JevID != "resp-1" || dec.Provider != "typesafe" {
		t.Fatalf("meta not copied: %+v", dec)
	}
	if dec.Usage == nil || dec.Usage.InputTokens != 10 {
		t.Fatalf("usage not copied: %+v", dec.Usage)
	}
	if dec.Difficulty != "easy" || dec.ClaudeFallback != "haiku" {
		t.Fatalf("difficulty %q claude %q", dec.Difficulty, dec.ClaudeFallback)
	}
	if len(dec.Candidates) != 3 {
		t.Fatalf("candidates %v", dec.Candidates)
	}
}

func TestDecidePickTieChoiceWins(t *testing.T) {
	p := validJevPolicy()
	live := p.Candidates
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("b", map[string]float64{"a": 0.5, "b": 0.5, "c": 0.0}),
		},
	}
	dec := DecidePick(p, live, resp, nil)
	// "b" is second in policy order but equals the choice, so it wins the tie.
	if dec.Primary.Key != "b" {
		t.Fatalf("primary %q want b (choice wins tie)", dec.Primary.Key)
	}
	if dec.Backup == nil || dec.Backup.Key != "a" {
		t.Fatalf("backup %+v want a", dec.Backup)
	}
}

func TestDecidePickTiePolicyOrder(t *testing.T) {
	p := validJevPolicy()
	live := p.Candidates
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("c", map[string]float64{"a": 0.5, "b": 0.5, "c": 0.1}),
		},
	}
	dec := DecidePick(p, live, resp, nil)
	// a and b tie at 0.5 and neither is the choice; policy order keeps a first.
	if dec.Primary.Key != "a" || dec.Backup == nil || dec.Backup.Key != "b" {
		t.Fatalf("primary/backup %q/%v want a/b", dec.Primary.Key, dec.Backup)
	}
}

func TestDecidePickChoiceNotLive(t *testing.T) {
	p := validJevPolicy()
	live := p.Candidates[:2] // a, b live; c dropped
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("c", nil),
		},
	}
	dec := DecidePick(p, live, resp, nil)
	if dec.Reason != "invalid_response" {
		t.Fatalf("reason %q want invalid_response", dec.Reason)
	}
	if dec.Error == nil || !strings.Contains(dec.Error.Message, "not a live candidate") {
		t.Fatalf("error %+v want 'not a live candidate'", dec.Error)
	}
	if dec.Primary.Key != "a" || dec.Backup == nil || dec.Backup.Key != "b" {
		t.Fatalf("fallback %q/%v want a/b", dec.Primary.Key, dec.Backup)
	}
}

func TestDecidePickJevError(t *testing.T) {
	p := validJevPolicy()
	p.FallbackPair = []string{"a", "z"} // requires a 4th candidate "z"
	p.Candidates = append(p.Candidates, JevCandidate{Key: "z", Model: "vendor/model-z", What: "spare"})
	live := p.Candidates[1:3] // b, c live; a and z dropped
	jevErr := &JevError{Message: "upstream failed", Retryable: true}
	dec := DecidePick(p, live, nil, jevErr)
	if dec.Reason != "error" || dec.Error != jevErr {
		t.Fatalf("reason/error %q/%v want error/jevErr", dec.Reason, dec.Error)
	}
	// Fallback pair a,z are not live; fill from live in policy order: b, c.
	if dec.Primary.Key != "b" || dec.Backup == nil || dec.Backup.Key != "c" {
		t.Fatalf("fallback %q/%v want b/c", dec.Primary.Key, dec.Backup)
	}
}

func TestDecidePickEmptyProbabilities(t *testing.T) {
	p := validJevPolicy()
	live := p.Candidates
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("b", nil),
		},
	}
	dec := DecidePick(p, live, resp, nil)
	if dec.Primary.Key != "b" || dec.Primary.Probability != nil {
		t.Fatalf("primary %+v want b with nil probability", dec.Primary)
	}
	// FallbackPair ["a","b"]: first entry != b that is live is "a".
	if dec.Backup == nil || dec.Backup.Key != "a" {
		t.Fatalf("backup %+v want a", dec.Backup)
	}
}

func TestDecidePickNoLiveCandidates(t *testing.T) {
	p := validJevPolicy()
	dec := DecidePick(p, nil, nil, nil)
	if dec.Reason != "invalid_response" {
		t.Fatalf("reason %q want invalid_response", dec.Reason)
	}
	if dec.Error == nil || dec.Error.Message != "no live candidates" {
		t.Fatalf("error %+v want 'no live candidates'", dec.Error)
	}
	if dec.Difficulty != "easy" || dec.ClaudeFallback != "haiku" {
		t.Fatalf("difficulty %q claude %q want easy/haiku", dec.Difficulty, dec.ClaudeFallback)
	}
}

func TestDecidePickInvalidResponseKeepsBilling(t *testing.T) {
	p := validJevPolicy()
	live := p.Candidates
	resp := &JevResponse{
		Model:    "classifier-v1",
		ID:       "resp-9",
		Provider: "typesafe",
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("zz", nil),
		},
		Usage: JevUsage{InputTokens: 5, OutputTokens: 1},
	}
	dec := DecidePick(p, live, resp, nil)
	if dec.Reason != "invalid_response" {
		t.Fatalf("reason %q want invalid_response", dec.Reason)
	}
	if dec.JevModel != "classifier-v1" || dec.JevID != "resp-9" || dec.Provider != "typesafe" {
		t.Fatalf("billing meta not kept: %+v", dec)
	}
	if dec.Usage == nil || dec.Usage.InputTokens != 5 {
		t.Fatalf("usage not kept: %+v", dec.Usage)
	}
}

func TestDecidePickDifficultyFallbackToDefault(t *testing.T) {
	p := validJevPolicy()
	live := p.Candidates
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("a", map[string]float64{"a": 1.0}),
			"difficulty":   JevChoiceAnswer{Type: "choice", Choice: "nonexistent"},
		},
	}
	dec := DecidePick(p, live, resp, nil)
	if dec.Difficulty != "easy" || dec.ClaudeFallback != "haiku" {
		t.Fatalf("difficulty %q claude %q want easy/haiku", dec.Difficulty, dec.ClaudeFallback)
	}
}

func TestDecidePickPairedBackup(t *testing.T) {
	p := validJevPolicy()
	p.Candidates[0].Backup = &JevBackup{Key: "a-backup", Model: "vendor/model-a2"}
	live := p.Candidates
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("a", map[string]float64{"a": 0.6, "b": 0.3, "c": 0.1}),
		},
	}
	dec := DecidePick(p, live, resp, nil)
	if dec.Primary.Key != "a" {
		t.Fatalf("primary %q want a", dec.Primary.Key)
	}
	if dec.Backup == nil || dec.Backup.Key != "a-backup" || dec.Backup.Model != "vendor/model-a2" {
		t.Fatalf("backup %+v want paired a-backup", dec.Backup)
	}
	if dec.Backup.Probability != nil {
		t.Fatalf("paired backup must have no probability, got %v", dec.Backup.Probability)
	}
	if dec.BackupSource != "paired" {
		t.Fatalf("backup_source %q want paired", dec.BackupSource)
	}
}

func TestDecidePickPairedBackupEmptyProbabilities(t *testing.T) {
	p := validJevPolicy()
	p.Candidates[0].Backup = &JevBackup{Key: "a-backup", Model: "vendor/model-a2"}
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("a", nil),
		},
	}
	dec := DecidePick(p, p.Candidates, resp, nil)
	if dec.Backup == nil || dec.Backup.Key != "a-backup" {
		t.Fatalf("backup %+v want paired a-backup", dec.Backup)
	}
	if dec.BackupSource != "paired" {
		t.Fatalf("backup_source %q want paired", dec.BackupSource)
	}
}

func TestDecidePickBackupSourceValues(t *testing.T) {
	t.Run("rank2", func(t *testing.T) {
		p := validJevPolicy()
		resp := &JevResponse{
			Answers: map[string]JevChoiceAnswer{
				"target_model": choiceAnswer("b", map[string]float64{"a": 0.2, "b": 0.7, "c": 0.1}),
			},
		}
		dec := DecidePick(p, p.Candidates, resp, nil)
		if dec.BackupSource != "rank2" {
			t.Fatalf("backup_source %q want rank2", dec.BackupSource)
		}
	})
	t.Run("fallback_pair", func(t *testing.T) {
		p := validJevPolicy()
		dec := DecidePick(p, p.Candidates, nil, &JevError{Message: "boom"})
		if dec.BackupSource != "fallback_pair" {
			t.Fatalf("backup_source %q want fallback_pair", dec.BackupSource)
		}
	})
	t.Run("none", func(t *testing.T) {
		p := validJevPolicy()
		live := p.Candidates[:1]
		resp := &JevResponse{
			Answers: map[string]JevChoiceAnswer{
				"target_model": choiceAnswer("a", map[string]float64{"a": 1.0}),
			},
		}
		dec := DecidePick(p, live, resp, nil)
		if dec.BackupSource != "none" {
			t.Fatalf("backup_source %q want none", dec.BackupSource)
		}
	})
}

// validJevPolicy3Level returns a policy with three ordered difficulty levels
// (trivial < moderate < hard) and candidate ceilings a=moderate, b=hard, c=hard.
func validJevPolicy3Level() JevRouterPolicy {
	p := validJevPolicy()
	p.Difficulty = []JevDifficultyLevel{
		{Key: "trivial", Description: "trivial task", ClaudeModel: "haiku"},
		{Key: "moderate", Description: "moderate task", ClaudeModel: "sonnet"},
		{Key: "hard", Description: "hard task", ClaudeModel: "opus"},
	}
	p.DefaultDifficulty = "trivial"
	p.Candidates[0].MaxDifficulty = "moderate"
	p.Candidates[1].MaxDifficulty = "hard"
	p.Candidates[2].MaxDifficulty = "hard"
	return p
}

func TestDecidePickCeilingApplied(t *testing.T) {
	p := validJevPolicy3Level()
	p.Candidates[1].Backup = &JevBackup{Key: "b-backup", Model: "vendor/model-b2"}
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("a", map[string]float64{"a": 0.6, "b": 0.3, "c": 0.1}),
			"difficulty":   choiceAnswer("hard", nil),
		},
	}
	dec := DecidePick(p, p.Candidates, resp, nil)
	if !dec.CeilingApplied {
		t.Fatal("ceiling_applied must be true when the Jev choice is below the difficulty ceiling")
	}
	if dec.JevChoice != "a" {
		t.Fatalf("jev_choice %q want a (raw choice unchanged)", dec.JevChoice)
	}
	if dec.Primary.Key != "b" {
		t.Fatalf("primary %q want most probable eligible candidate b", dec.Primary.Key)
	}
	if dec.Backup == nil || dec.Backup.Key != "b-backup" {
		t.Fatalf("backup %+v want paired b-backup", dec.Backup)
	}
	if dec.BackupSource != "paired" {
		t.Fatalf("backup_source %q want paired", dec.BackupSource)
	}
	if len(dec.Eligible) != 2 || dec.Eligible[0] != "b" || dec.Eligible[1] != "c" {
		t.Fatalf("eligible %v want [b c]", dec.Eligible)
	}
}

func TestDecidePickCeilingTrivialNoFilter(t *testing.T) {
	p := validJevPolicy3Level()
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("a", map[string]float64{"a": 0.6, "b": 0.3, "c": 0.1}),
			"difficulty":   choiceAnswer("trivial", nil),
		},
	}
	dec := DecidePick(p, p.Candidates, resp, nil)
	if dec.CeilingApplied {
		t.Fatal("ceiling_applied must be false for the easiest difficulty")
	}
	if dec.Primary.Key != "a" {
		t.Fatalf("primary %q want a", dec.Primary.Key)
	}
	if len(dec.Eligible) != 3 {
		t.Fatalf("eligible %v want all 3 candidates", dec.Eligible)
	}
}

func TestDecidePickCeilingAllBelowNoFilter(t *testing.T) {
	p := validJevPolicy3Level()
	for i := range p.Candidates {
		p.Candidates[i].MaxDifficulty = "trivial"
	}
	resp := &JevResponse{
		Answers: map[string]JevChoiceAnswer{
			"target_model": choiceAnswer("a", map[string]float64{"a": 0.6, "b": 0.3, "c": 0.1}),
			"difficulty":   choiceAnswer("hard", nil),
		},
	}
	dec := DecidePick(p, p.Candidates, resp, nil)
	if dec.CeilingApplied {
		t.Fatal("ceiling_applied must be false when the filter removes everyone")
	}
	if dec.Primary.Key != "a" {
		t.Fatalf("primary %q want a (no filter)", dec.Primary.Key)
	}
	if len(dec.Eligible) != 3 {
		t.Fatalf("eligible %v want all 3 candidates", dec.Eligible)
	}
}
