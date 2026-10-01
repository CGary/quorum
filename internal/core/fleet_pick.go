package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"
)

// knownJevProviders is the closed set of classifier backends a Jev router policy
// may name. It is data, not code truth: adding a provider is a policy change,
// never a recompile.
var knownJevProviders = map[string]bool{
	"typesafe":             true,
	"openrouter_decisions": true,
	"openrouter_systemone": true,
}

// JevRouterPolicy is the caller-supplied Jev (classifier) routing policy. It
// carries the target classifier provider, its connection settings, the two
// questions asked of the classifier (target_model choice + difficulty choice),
// the candidate list, and the fallback pair used when the classifier cannot be
// reached or returns an unusable answer.
type JevRouterPolicy struct {
	Version                int                          `json:"version" yaml:"version"`
	Provider               string                       `json:"provider" yaml:"provider"`
	FallbackProvider       string                       `json:"fallback_provider" yaml:"fallback_provider"`
	Providers              map[string]JevProviderConfig `json:"providers" yaml:"providers"`
	TimeoutS               int                          `json:"timeout_s" yaml:"timeout_s"`
	MaxStateTokens         int                          `json:"max_state_tokens" yaml:"max_state_tokens"`
	Transport              string                       `json:"transport" yaml:"transport"`
	Instructions           string                       `json:"instructions" yaml:"instructions"`
	DifficultyInstructions string                       `json:"difficulty_instructions" yaml:"difficulty_instructions"`
	Candidates             []JevCandidate               `json:"candidates" yaml:"candidates"`
	Difficulty             []JevDifficultyLevel         `json:"difficulty" yaml:"difficulty"`
	DefaultDifficulty      string                       `json:"default_difficulty" yaml:"default_difficulty"`
	FallbackPair           []string                     `json:"fallback_pair" yaml:"fallback_pair"`
}

// JevProviderConfig is the connection settings for one classifier backend.
// APIKeyEnv names an environment variable that holds the secret; the secret
// value itself is never embedded in policy.
type JevProviderConfig struct {
	URL       string `json:"url" yaml:"url"`
	Model     string `json:"model" yaml:"model"`
	APIKeyEnv string `json:"api_key_env" yaml:"api_key_env"`
}

// JevCandidate is one target_model option offered to the classifier. Key is the
// classifier-facing choice value; Model is the concrete model it resolves to.
// What/NotFor/Examples are the classifier prompt guidance; Evidence is
// human-facing and is never sent to the classifier. CostTier is the candidate's
// advisory cost band (low|medium|high; empty when unset) and Backup is the
// optional paired fallback cell attached to this candidate.
type JevCandidate struct {
	Key           string     `json:"key" yaml:"key"`
	Model         string     `json:"model" yaml:"model"`
	What          string     `json:"what" yaml:"what"`
	NotFor        string     `json:"not_for" yaml:"not_for"`
	Examples      []string   `json:"examples" yaml:"examples"`
	Evidence      []string   `json:"evidence" yaml:"evidence"`
	CostTier      string     `json:"cost_tier" yaml:"cost_tier"`
	MaxDifficulty string     `json:"max_difficulty" yaml:"max_difficulty"`
	Backup        *JevBackup `json:"backup,omitempty" yaml:"backup,omitempty"`
}

// JevBackup is a paired fallback cell attached to a candidate: when Jev picks
// that candidate as primary, this cell becomes the decision's backup without a
// second classifier call. Key is unique among all candidates and backups; Model
// must differ from the candidate's own model. Evidence is human-facing only.
type JevBackup struct {
	Key      string   `json:"key" yaml:"key"`
	Model    string   `json:"model" yaml:"model"`
	Evidence []string `json:"evidence" yaml:"evidence"`
}

// JevDifficultyLevel is one difficulty choice offered to the classifier and the
// internal Claude role it maps to when a human fallback is needed.
type JevDifficultyLevel struct {
	Key         string `json:"key" yaml:"key"`
	Description string `json:"description" yaml:"description"`
	ClaudeModel string `json:"claude_model" yaml:"claude_model"`
}

// ValidateJevRouterPolicy checks the structural invariants of a Jev policy and
// returns the first violation as an error whose message names the offending
// field path. It performs no I/O and never inspects secret values.
func ValidateJevRouterPolicy(p JevRouterPolicy) error {
	if !knownJevProviders[p.Provider] {
		return fmt.Errorf("provider: %q is not a known Jev provider", p.Provider)
	}
	cfg, ok := p.Providers[p.Provider]
	if !ok {
		return fmt.Errorf("providers: provider %q has no entry", p.Provider)
	}
	if cfg.URL == "" {
		return fmt.Errorf("providers.%s.url: must not be empty", p.Provider)
	}
	if cfg.Model == "" {
		return fmt.Errorf("providers.%s.model: must not be empty", p.Provider)
	}
	if cfg.APIKeyEnv == "" {
		return fmt.Errorf("providers.%s.api_key_env: must not be empty", p.Provider)
	}
	if p.FallbackProvider != "" {
		if p.FallbackProvider == p.Provider {
			return fmt.Errorf("fallback_provider: must differ from provider")
		}
		if !knownJevProviders[p.FallbackProvider] {
			return fmt.Errorf("fallback_provider: %q is not a known Jev provider", p.FallbackProvider)
		}
		fcfg, ok := p.Providers[p.FallbackProvider]
		if !ok {
			return fmt.Errorf("fallback_provider: provider %q has no entry", p.FallbackProvider)
		}
		if fcfg.URL == "" {
			return fmt.Errorf("fallback_provider: providers.%s.url must not be empty", p.FallbackProvider)
		}
		if fcfg.Model == "" {
			return fmt.Errorf("fallback_provider: providers.%s.model must not be empty", p.FallbackProvider)
		}
		if fcfg.APIKeyEnv == "" {
			return fmt.Errorf("fallback_provider: providers.%s.api_key_env must not be empty", p.FallbackProvider)
		}
	}
	if p.TimeoutS <= 0 {
		return fmt.Errorf("timeout_s: must be > 0")
	}
	if p.MaxStateTokens <= 0 {
		return fmt.Errorf("max_state_tokens: must be > 0")
	}
	if p.Transport == "" {
		return fmt.Errorf("transport: must not be empty")
	}
	if p.Instructions == "" {
		return fmt.Errorf("instructions: must not be empty")
	}
	if p.DifficultyInstructions == "" {
		return fmt.Errorf("difficulty_instructions: must not be empty")
	}
	if len(p.Candidates) == 0 {
		return fmt.Errorf("candidates: must not be empty")
	}
	if len(p.Candidates) > 255 {
		return fmt.Errorf("candidates: must not exceed 255 entries")
	}
	seenKeys := make(map[string]bool, len(p.Candidates))
	seenModels := make(map[string]bool, len(p.Candidates))
	for i, c := range p.Candidates {
		if c.Key == "" {
			return fmt.Errorf("candidates[%d].key: must not be empty", i)
		}
		if seenKeys[c.Key] {
			return fmt.Errorf("candidates[%d].key: duplicate key %q", i, c.Key)
		}
		seenKeys[c.Key] = true
		if c.Model == "" {
			return fmt.Errorf("candidates[%d].model: must not be empty", i)
		}
		if seenModels[c.Model] {
			return fmt.Errorf("candidates[%d].model: duplicate model %q", i, c.Model)
		}
		seenModels[c.Model] = true
		if c.What == "" {
			return fmt.Errorf("candidates[%d].what: must not be empty", i)
		}
		switch c.CostTier {
		case "", "low", "medium", "high":
		default:
			return fmt.Errorf("candidates[%d].cost_tier: %q not in {low,medium,high}", i, c.CostTier)
		}
	}
	seenBackupKeys := make(map[string]bool)
	seenBackupModels := make(map[string]bool)
	for i, c := range p.Candidates {
		if c.Backup == nil {
			continue
		}
		if c.Backup.Key == "" {
			return fmt.Errorf("candidates[%d].backup.key: must not be empty", i)
		}
		if c.Backup.Model == "" {
			return fmt.Errorf("candidates[%d].backup.model: must not be empty", i)
		}
		if c.Backup.Model == c.Model {
			return fmt.Errorf("candidates[%d].backup.model: must differ from candidate model", i)
		}
		if seenKeys[c.Backup.Key] {
			return fmt.Errorf("candidates[%d].backup.key: collides with candidate key %q", i, c.Backup.Key)
		}
		if seenBackupKeys[c.Backup.Key] {
			return fmt.Errorf("candidates[%d].backup.key: duplicate backup key %q", i, c.Backup.Key)
		}
		seenBackupKeys[c.Backup.Key] = true
		if seenBackupModels[c.Backup.Model] {
			return fmt.Errorf("candidates[%d].backup.model: duplicate backup model %q", i, c.Backup.Model)
		}
		seenBackupModels[c.Backup.Model] = true
	}
	if len(p.Difficulty) < 2 {
		return fmt.Errorf("difficulty: must have at least 2 levels")
	}
	seenDiff := make(map[string]bool, len(p.Difficulty))
	for i, d := range p.Difficulty {
		if d.Key == "" {
			return fmt.Errorf("difficulty[%d].key: must not be empty", i)
		}
		if seenDiff[d.Key] {
			return fmt.Errorf("difficulty[%d].key: duplicate key %q", i, d.Key)
		}
		seenDiff[d.Key] = true
		switch d.ClaudeModel {
		case "haiku", "sonnet", "opus":
		default:
			return fmt.Errorf("difficulty[%d].claude_model: %q not in {haiku,sonnet,opus}", i, d.ClaudeModel)
		}
	}
	for i, c := range p.Candidates {
		if !seenDiff[c.MaxDifficulty] {
			return fmt.Errorf("candidates[%d].max_difficulty: %q is not a difficulty key", i, c.MaxDifficulty)
		}
	}
	if !seenDiff[p.DefaultDifficulty] {
		return fmt.Errorf("default_difficulty: %q is not a difficulty key", p.DefaultDifficulty)
	}
	if len(p.FallbackPair) != 2 {
		return fmt.Errorf("fallback_pair: must have exactly 2 entries")
	}
	if p.FallbackPair[0] == p.FallbackPair[1] {
		return fmt.Errorf("fallback_pair: entries must be distinct")
	}
	for i, k := range p.FallbackPair {
		if !seenKeys[k] {
			return fmt.Errorf("fallback_pair[%d]: %q is not a candidate key", i, k)
		}
	}
	return nil
}

// DroppedCandidate records a policy candidate excluded from the live set and the
// reason it was dropped.
type DroppedCandidate struct {
	Key    string `json:"key"`
	Model  string `json:"model"`
	Reason string `json:"reason"`
}

// FilterLiveCandidates reduces the policy candidate list (preserving policy
// order) to those still eligible: the transport is active, the candidate's model
// is in the transport's live catalog, and the kill-switch control state does not
// disable either the whole transport or that specific model.
func FilterLiveCandidates(p JevRouterPolicy, transportActive bool, transportModels map[string]bool, control ControlState) (live []JevCandidate, dropped []DroppedCandidate) {
	if !transportActive {
		for _, c := range p.Candidates {
			dropped = append(dropped, DroppedCandidate{Key: c.Key, Model: c.Model, Reason: "transport_inactive"})
		}
		return live, dropped
	}
	for _, c := range p.Candidates {
		if isJevKillSwitched(p.Transport, c.Model, control) {
			dropped = append(dropped, DroppedCandidate{Key: c.Key, Model: c.Model, Reason: "kill_switch"})
			continue
		}
		if !transportModels[c.Model] {
			dropped = append(dropped, DroppedCandidate{Key: c.Key, Model: c.Model, Reason: "not_in_catalog"})
			continue
		}
		if c.Backup != nil {
			if isJevKillSwitched(p.Transport, c.Backup.Model, control) {
				dropped = append(dropped, DroppedCandidate{Key: c.Backup.Key, Model: c.Backup.Model, Reason: "backup_kill_switch"})
				c.Backup = nil
			} else if !transportModels[c.Backup.Model] {
				dropped = append(dropped, DroppedCandidate{Key: c.Backup.Key, Model: c.Backup.Model, Reason: "backup_not_in_catalog"})
				c.Backup = nil
			}
		}
		live = append(live, c)
	}
	return live, dropped
}

// isJevKillSwitched reports whether the control state disables the transport as
// a whole or the exact transport+"/"+model identity.
func isJevKillSwitched(transport, model string, control ControlState) bool {
	full := transport + "/" + model
	for _, e := range control.Disabled {
		if e.Target == transport || e.Target == full {
			return true
		}
	}
	return false
}

// JevQuestion is one multiple-choice question posed to the classifier: a type
// (currently always "choice"), the instructions, and the criteria keyed by the
// choice value.
type JevQuestion struct {
	Type         string         `json:"type"`
	Instructions string         `json:"instructions"`
	Criteria     map[string]any `json:"criteria"`
}

// JevRequest is the classifier request body: the classifier model to call, the
// state (prompt + optional context), and the questions.
type JevRequest struct {
	Model     string                 `json:"model"`
	State     map[string]string      `json:"state"`
	Questions map[string]JevQuestion `json:"questions"`
}

// EstimateJevTokens approximates the classifier token cost of a request as the
// byte length of its JSON encoding divided by three. It is deliberately
// conservative because Jev exposes no public tokenizer.
func EstimateJevTokens(req JevRequest) int {
	b, err := json.Marshal(req)
	if err != nil {
		return 0
	}
	return (len(b) + 2) / 3
}

// ErrJevStateTooLarge is returned when the prompt alone, with no context, still
// exceeds the policy max_state_tokens budget and therefore cannot be trimmed.
var ErrJevStateTooLarge = errors.New("jev request state exceeds max_state_tokens")

// BuildJevRequest assembles the classifier request for a provider from the live
// candidates, prompt and optional context. When the request exceeds the token
// budget it trims context from the start (keeping the most recent tail) until it
// fits, setting trimmed; the prompt is never trimmed, and a prompt that alone
// exceeds the budget yields ErrJevStateTooLarge.
func BuildJevRequest(p JevRouterPolicy, provider string, live []JevCandidate, prompt, context string) (req JevRequest, trimmed bool, estTokens int, err error) {
	req = JevRequest{
		Model: p.Providers[provider].Model,
		State: map[string]string{"prompt": prompt},
		Questions: map[string]JevQuestion{
			"target_model": {
				Type:         "choice",
				Instructions: p.Instructions,
				Criteria:     buildTargetModelCriteria(live),
			},
			"difficulty": {
				Type:         "choice",
				Instructions: p.DifficultyInstructions,
				Criteria:     buildDifficultyCriteria(p.Difficulty),
			},
		},
	}
	if context != "" {
		req.State["context"] = context
	}
	est := EstimateJevTokens(req)
	if est <= p.MaxStateTokens {
		return req, false, est, nil
	}

	trimmed = true
	promptOnly := JevRequest{Model: req.Model, State: map[string]string{"prompt": prompt}, Questions: req.Questions}
	if EstimateJevTokens(promptOnly) > p.MaxStateTokens {
		return JevRequest{}, true, EstimateJevTokens(promptOnly), ErrJevStateTooLarge
	}

	ctx := context
	for {
		if ctx == "" {
			delete(req.State, "context")
			return req, true, EstimateJevTokens(req), nil
		}
		req.State["context"] = ctx
		est = EstimateJevTokens(req)
		if est <= p.MaxStateTokens {
			return req, true, est, nil
		}
		step := (est - p.MaxStateTokens) * 3
		if step < 1 {
			step = 1
		}
		if step >= len(ctx) {
			ctx = ""
		} else {
			// Advance to a rune boundary so the kept tail stays valid UTF-8.
			for step < len(ctx) && !utf8.RuneStart(ctx[step]) {
				step++
			}
			ctx = ctx[step:]
		}
	}
}

// buildTargetModelCriteria maps each live candidate key to its classifier
// guidance. Evidence is deliberately omitted: it is human-facing and must never
// reach the classifier. NotFor and Examples are omitted when empty.
func buildTargetModelCriteria(live []JevCandidate) map[string]any {
	crit := make(map[string]any, len(live))
	for _, c := range live {
		entry := map[string]any{"what": c.What}
		if c.NotFor != "" {
			entry["not_for"] = c.NotFor
		}
		if len(c.Examples) > 0 {
			entry["examples"] = c.Examples
		}
		if c.CostTier != "" {
			entry["cost"] = c.CostTier
		}
		crit[c.Key] = entry
	}
	return crit
}

// buildDifficultyCriteria maps each difficulty key to its plain-text
// description.
func buildDifficultyCriteria(levels []JevDifficultyLevel) map[string]any {
	crit := make(map[string]any, len(levels))
	for _, l := range levels {
		crit[l.Key] = l.Description
	}
	return crit
}

// JevUsage is the classifier token/cost usage block.
type JevUsage struct {
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	Cost         *float64 `json:"cost,omitempty"`
}

// JevChoiceAnswer is one classifier answer: its type, the chosen value, and
// optionally the per-option probabilities and a confidence score.
type JevChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// JevResponse is the classifier response body. Provider may arrive as a raw
// string or (from OpenRouter) an object; ParseJevResponse normalizes it to the
// raw string form.
type JevResponse struct {
	Model    string                     `json:"model"`
	ID       string                     `json:"id,omitempty"`
	Provider string                     `json:"provider,omitempty"`
	Answers  map[string]JevChoiceAnswer `json:"answers"`
	Usage    JevUsage                   `json:"usage"`
}

// ParseJevResponse decodes a classifier response. It first tries the natural
// string form of the "provider" field; if that fails because OpenRouter returns
// the field as an object, it re-decodes the field as raw JSON and keeps the raw
// string form instead.
func ParseJevResponse(body []byte) (*JevResponse, error) {
	var resp JevResponse
	if err := json.Unmarshal(body, &resp); err == nil {
		return &resp, nil
	}
	var raw struct {
		Model    string                     `json:"model"`
		ID       string                     `json:"id,omitempty"`
		Provider json.RawMessage            `json:"provider,omitempty"`
		Answers  map[string]JevChoiceAnswer `json:"answers"`
		Usage    JevUsage                   `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return &JevResponse{
		Model:    raw.Model,
		ID:       raw.ID,
		Provider: string(raw.Provider),
		Answers:  raw.Answers,
		Usage:    raw.Usage,
	}, nil
}

// JevError is a normalized classifier error. Message never contains the API key
// or any other secret.
type JevError struct {
	Status    int    `json:"status,omitempty"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Error implements the error interface, returning the message.
func (e *JevError) Error() string {
	return e.Message
}

// JevErrorFallsBack reports whether a Jev error should trigger the fallback
// provider. Request-shaped problems that would fail identically on another
// provider (HTTP 400, 413, 422) do NOT fall back; every other failure —
// transport errors, timeouts, 401/402/403/404, 429, 5xx, and unparseable 200
// bodies — does.
func JevErrorFallsBack(e *JevError) bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case 400, 413, 422:
		return false
	default:
		return true
	}
}

// PickCell is one resolved executor cell: the candidate key, its model, and
// optionally the classifier-assigned probability.
type PickCell struct {
	Key         string   `json:"key"`
	Model       string   `json:"model"`
	Probability *float64 `json:"probability,omitempty"`
}

// ProviderAttempt records one provider in the ordered chain that failed before
// an answer was obtained (or before the pick failed open).
type ProviderAttempt struct {
	Provider  string `json:"provider"`
	Status    int    `json:"status,omitempty"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
}

// PickDecision is the final routing decision derived from a classifier answer.
// Primary/Backup are the chosen cells; ClaudeFallback is the internal Claude
// role to use when the pick must degrade to a human/subagent. Dropped,
// StateTrimmed, EstimatedTokens and LatencyMS are attached by the orchestrator
// from the surrounding request lifecycle, not by DecidePick.
type PickDecision struct {
	Primary                 PickCell           `json:"primary"`
	Backup                  *PickCell          `json:"backup,omitempty"`
	ClaudeFallback          string             `json:"claude_fallback"`
	Difficulty              string             `json:"difficulty"`
	Reason                  string             `json:"reason"`
	JevChoice               string             `json:"jev_choice,omitempty"`
	Confidence              *float64           `json:"confidence,omitempty"`
	Probabilities           map[string]float64 `json:"probabilities,omitempty"`
	DifficultyConfidence    *float64           `json:"difficulty_confidence,omitempty"`
	DifficultyProbabilities map[string]float64 `json:"difficulty_probabilities,omitempty"`
	BackupSource            string             `json:"backup_source"`
	JevModel                string             `json:"jev_model,omitempty"`
	JevID                   string             `json:"jev_id,omitempty"`
	Provider                string             `json:"provider,omitempty"`
	UpstreamProvider        string             `json:"upstream_provider,omitempty"`
	ProviderAttempts        []ProviderAttempt  `json:"provider_attempts,omitempty"`
	Usage                   *JevUsage          `json:"usage,omitempty"`
	LatencyMS               int64              `json:"latency_ms"`
	Error                   *JevError          `json:"error,omitempty"`
	Candidates              []string           `json:"candidates"`
	Dropped                 []DroppedCandidate `json:"dropped"`
	CeilingApplied          bool               `json:"ceiling_applied"`
	Eligible                []string           `json:"eligible,omitempty"`
	StateTrimmed            bool               `json:"state_trimmed"`
	EstimatedTokens         int                `json:"estimated_tokens"`
}

// DecidePick turns a classifier response (or its error) into a PickDecision.
// len(live) must be >= 1. Difficulty is always resolved — from the classifier
// answer when valid, otherwise the policy default — even on the error path. On
// a successful "choice" answer it ranks live candidates by probability; on
// error/invalid responses it degrades to the fallback pair padded with live
// candidates in policy order.
func DecidePick(p JevRouterPolicy, live []JevCandidate, resp *JevResponse, jevErr *JevError) PickDecision {
	liveByKey := make(map[string]JevCandidate, len(live))
	candidates := make([]string, 0, len(live))
	for _, c := range live {
		liveByKey[c.Key] = c
		candidates = append(candidates, c.Key)
	}

	dec := PickDecision{Candidates: candidates}

	difficultyKey := p.DefaultDifficulty
	var diffConf *float64
	var diffProbs map[string]float64
	if resp != nil {
		if a, ok := resp.Answers["difficulty"]; ok && a.Type == "choice" && isJevDifficultyKey(p, a.Choice) {
			difficultyKey = a.Choice
			diffConf = a.Confidence
			diffProbs = a.Probabilities
		}
	}
	dec.Difficulty = difficultyKey
	dec.ClaudeFallback = claudeModelForDifficulty(p, difficultyKey)
	dec.DifficultyConfidence = diffConf
	dec.DifficultyProbabilities = diffProbs

	if resp != nil {
		dec.JevModel = resp.Model
		dec.JevID = resp.ID
		dec.UpstreamProvider = resp.Provider
		if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 || resp.Usage.Cost != nil {
			u := resp.Usage
			dec.Usage = &u
		}
	}

	if len(live) == 0 {
		dec.Reason = "invalid_response"
		dec.Error = &JevError{Message: "no live candidates"}
		dec.BackupSource = "none"
		return dec
	}

	if jevErr != nil {
		dec.Reason = "error"
		dec.Error = jevErr
		dec.Primary, dec.Backup = fallbackPick(p, live, liveByKey)
		dec.BackupSource = fallbackBackupSource(dec.Backup)
		return dec
	}
	if resp == nil {
		dec.Reason = "invalid_response"
		dec.Error = &JevError{Message: "empty response"}
		dec.Primary, dec.Backup = fallbackPick(p, live, liveByKey)
		dec.BackupSource = fallbackBackupSource(dec.Backup)
		return dec
	}
	a, ok := resp.Answers["target_model"]
	if !ok {
		dec.Reason = "invalid_response"
		dec.Error = &JevError{Message: "missing target_model answer"}
		dec.Primary, dec.Backup = fallbackPick(p, live, liveByKey)
		dec.BackupSource = fallbackBackupSource(dec.Backup)
		return dec
	}
	if a.Type != "choice" {
		dec.Reason = "invalid_response"
		dec.Error = &JevError{Message: "target_model answer is not a choice"}
		dec.Primary, dec.Backup = fallbackPick(p, live, liveByKey)
		dec.BackupSource = fallbackBackupSource(dec.Backup)
		return dec
	}
	if _, liveOK := liveByKey[a.Choice]; !liveOK {
		dec.Reason = "invalid_response"
		dec.Error = &JevError{Message: fmt.Sprintf("target_model choice %q is not a live candidate", a.Choice)}
		dec.Primary, dec.Backup = fallbackPick(p, live, liveByKey)
		dec.BackupSource = fallbackBackupSource(dec.Backup)
		return dec
	}

	dec.Reason = "jev"
	dec.JevChoice = a.Choice
	dec.Confidence = a.Confidence
	dec.Probabilities = a.Probabilities

	eligible, ceilingApplied := applyDifficultyCeiling(p, live, difficultyKey, a.Choice)
	dec.CeilingApplied = ceilingApplied
	dec.Eligible = candidateKeys(eligible)

	eligibleByKey := make(map[string]JevCandidate, len(eligible))
	for _, c := range eligible {
		eligibleByKey[c.Key] = c
	}

	if len(a.Probabilities) == 0 {
		primary, ok := eligibleByKey[a.Choice]
		if !ok {
			primary = eligible[0]
		}
		dec.Primary = PickCell{Key: primary.Key, Model: primary.Model}
		dec.Backup, dec.BackupSource = resolveJevBackup(primary, backupFromFallbackOrPolicy(p, eligible, eligibleByKey, primary.Key))
	} else {
		ranked := rankCandidatesByProbability(eligible, a.Choice, a.Probabilities)
		dec.Primary = PickCell{Key: ranked[0].c.Key, Model: ranked[0].c.Model, Probability: floatPtr(ranked[0].p)}
		var rank2 *PickCell
		if len(ranked) > 1 {
			rank2 = &PickCell{Key: ranked[1].c.Key, Model: ranked[1].c.Model, Probability: floatPtr(ranked[1].p)}
		}
		dec.Backup, dec.BackupSource = resolveJevBackup(ranked[0].c, rank2)
	}

	return dec
}

// fallbackBackupSource labels the backup source on error/invalid_response
// paths: "fallback_pair" when a backup cell exists, "none" otherwise.
func fallbackBackupSource(backup *PickCell) string {
	if backup == nil {
		return "none"
	}
	return "fallback_pair"
}

// resolveJevBackup picks the Jev-path backup: a paired backup on the primary
// wins outright ("paired"); otherwise the rank-2 cell ("rank2"); otherwise no
// backup ("none").
func resolveJevBackup(primary JevCandidate, rank2 *PickCell) (*PickCell, string) {
	if primary.Backup != nil {
		return &PickCell{Key: primary.Backup.Key, Model: primary.Backup.Model}, "paired"
	}
	if rank2 != nil {
		return rank2, "rank2"
	}
	return nil, "none"
}

// isJevDifficultyKey reports whether key is a declared difficulty level.
func isJevDifficultyKey(p JevRouterPolicy, key string) bool {
	for _, d := range p.Difficulty {
		if d.Key == key {
			return true
		}
	}
	return false
}

// claudeModelForDifficulty returns the internal Claude role for a difficulty key.
func claudeModelForDifficulty(p JevRouterPolicy, key string) string {
	for _, d := range p.Difficulty {
		if d.Key == key {
			return d.ClaudeModel
		}
	}
	return ""
}

// difficultyIndex returns the ordered position of a difficulty key in the
// policy (index 0 is easiest), or -1 when the key is not a declared level.
func difficultyIndex(p JevRouterPolicy, key string) int {
	for i, d := range p.Difficulty {
		if d.Key == key {
			return i
		}
	}
	return -1
}

// applyDifficultyCeiling reduces the live candidates to those whose
// MaxDifficulty is at least as hard as the decided difficulty, preserving policy
// order. When the filter removes everyone it returns live unchanged (no ceiling
// effect). The boolean reports whether the Jev choice was excluded by the
// ceiling, i.e. the primary must differ from the raw Jev choice.
func applyDifficultyCeiling(p JevRouterPolicy, live []JevCandidate, difficulty, choice string) ([]JevCandidate, bool) {
	dIdx := difficultyIndex(p, difficulty)
	eligible := make([]JevCandidate, 0, len(live))
	for _, c := range live {
		if difficultyIndex(p, c.MaxDifficulty) >= dIdx {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return live, false
	}
	for _, c := range eligible {
		if c.Key == choice {
			return eligible, false
		}
	}
	return eligible, true
}

// candidateKeys returns the candidate keys in slice order.
func candidateKeys(candidates []JevCandidate) []string {
	keys := make([]string, 0, len(candidates))
	for _, c := range candidates {
		keys = append(keys, c.Key)
	}
	return keys
}

// fallbackPick builds the degraded pick: the fallback pair entries that are live
// in pair order, then the remaining live candidates in policy order, with no
// duplicates. Primary is the first; Backup is the second (nil when only one live
// candidate exists).
func fallbackPick(p JevRouterPolicy, live []JevCandidate, liveByKey map[string]JevCandidate) (PickCell, *PickCell) {
	ordered := make([]JevCandidate, 0, len(live))
	used := make(map[string]bool, len(live))
	for _, key := range p.FallbackPair {
		if c, ok := liveByKey[key]; ok && !used[key] {
			ordered = append(ordered, c)
			used[key] = true
		}
	}
	for _, c := range live {
		if !used[c.Key] {
			ordered = append(ordered, c)
		}
	}
	primary := PickCell{Key: ordered[0].Key, Model: ordered[0].Model}
	if len(ordered) < 2 {
		return primary, nil
	}
	return primary, &PickCell{Key: ordered[1].Key, Model: ordered[1].Model}
}

// backupFromFallbackOrPolicy returns the backup cell for the empty-probabilities
// path: the first live fallback pair entry that differs from the choice,
// otherwise the next live candidate in policy order.
func backupFromFallbackOrPolicy(p JevRouterPolicy, live []JevCandidate, liveByKey map[string]JevCandidate, choice string) *PickCell {
	for _, key := range p.FallbackPair {
		if key == choice {
			continue
		}
		if c, ok := liveByKey[key]; ok {
			return &PickCell{Key: c.Key, Model: c.Model}
		}
	}
	for _, c := range live {
		if c.Key != choice {
			return &PickCell{Key: c.Key, Model: c.Model}
		}
	}
	return nil
}

// rankedCandidate pairs a live candidate with its classifier probability.
type rankedCandidate struct {
	c JevCandidate
	p float64
}

// rankCandidatesByProbability ranks live candidates by probability descending.
// Ties are broken by the choice winning first, then by stable policy order —
// the API does not guarantee answer ordering.
func rankCandidatesByProbability(live []JevCandidate, choice string, probs map[string]float64) []rankedCandidate {
	ranked := make([]rankedCandidate, 0, len(live))
	for _, c := range live {
		ranked = append(ranked, rankedCandidate{c: c, p: probs[c.Key]})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].p != ranked[j].p {
			return ranked[i].p > ranked[j].p
		}
		iChoice := ranked[i].c.Key == choice
		jChoice := ranked[j].c.Key == choice
		if iChoice != jChoice {
			return iChoice
		}
		return false
	})
	return ranked
}
