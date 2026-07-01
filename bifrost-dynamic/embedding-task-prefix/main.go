package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

type pluginConfig struct {
	InputTypeParam string       `json:"input_type_param,omitempty"`
	StripInputType *bool        `json:"strip_input_type,omitempty"`
	SkipIfPrefixed *bool        `json:"skip_if_prefixed,omitempty"`
	Rules          []prefixRule `json:"rules,omitempty"`
}

type prefixRule struct {
	Name           string            `json:"name,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Model          string            `json:"model,omitempty"`
	ModelPrefix    string            `json:"model_prefix,omitempty"`
	InputTypeParam string            `json:"input_type_param,omitempty"`
	StripInputType *bool             `json:"strip_input_type,omitempty"`
	SkipIfPrefixed *bool             `json:"skip_if_prefixed,omitempty"`
	Prefixes       map[string]string `json:"prefixes,omitempty"`
}

var activeConfig pluginConfig

func Init(config any) error {
	activeConfig = pluginConfig{}
	if config == nil {
		return nil
	}

	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := json.Unmarshal(raw, &activeConfig); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	for i := range activeConfig.Rules {
		activeConfig.Rules[i].Provider = strings.TrimSpace(activeConfig.Rules[i].Provider)
		activeConfig.Rules[i].Model = strings.TrimSpace(activeConfig.Rules[i].Model)
		activeConfig.Rules[i].ModelPrefix = strings.TrimSpace(activeConfig.Rules[i].ModelPrefix)
		activeConfig.Rules[i].InputTypeParam = strings.TrimSpace(activeConfig.Rules[i].InputTypeParam)
		normalized := make(map[string]string, len(activeConfig.Rules[i].Prefixes))
		for key, value := range activeConfig.Rules[i].Prefixes {
			normKey := normalizeRole(key)
			if normKey == "" || value == "" {
				continue
			}
			normalized[normKey] = value
		}
		activeConfig.Rules[i].Prefixes = normalized
	}

	return nil
}

func GetName() string { return "embedding-task-prefix" }

func Cleanup() error {
	activeConfig = pluginConfig{}
	return nil
}

func PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	if req == nil || req.EmbeddingRequest == nil {
		return nil
	}

	embeddingReq := req.EmbeddingRequest
	if embeddingReq.Input == nil {
		return nil
	}

	rule, ok := findMatchingRule(embeddingReq.Provider, embeddingReq.Model)
	if !ok {
		return nil
	}

	inputTypeKey := effectiveInputTypeParam(rule)
	role := normalizeRole(readExtraParam(embeddingReq.Params, inputTypeKey))
	if role == "" {
		role = normalizeRole(readInputTypeFromRawBody(embeddingReq.RawRequestBody, inputTypeKey))
	}
	if role == "" {
		return nil
	}

	prefix, ok := rule.Prefixes[role]
	if !ok || prefix == "" {
		return nil
	}

	changed := applyPrefix(rule, embeddingReq.Input, prefix)
	if !changed {
		return nil
	}

	if shouldStripInputType(rule) && inputTypeKey != "" {
		stripExtraParam(embeddingReq, inputTypeKey)
	}

	logf(ctx, schemas.LogLevelInfo, "embedding-task-prefix applied %q role prefix for %s/%q", role, embeddingReq.Provider, embeddingReq.Model)
	return nil
}

func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

func findMatchingRule(provider schemas.ModelProvider, model string) (prefixRule, bool) {
	for _, rule := range activeConfig.Rules {
		if ruleMatches(rule, string(provider), model) {
			return rule, true
		}
	}
	return prefixRule{}, false
}

func ruleMatches(rule prefixRule, provider, model string) bool {
	if rule.Provider != "" && rule.Provider != "*" && !strings.EqualFold(rule.Provider, provider) {
		return false
	}
	if rule.Model != "" && rule.Model != model {
		return false
	}
	if rule.ModelPrefix != "" && !strings.HasPrefix(model, rule.ModelPrefix) {
		return false
	}
	return rule.Model != "" || rule.ModelPrefix != "" || rule.Provider != ""
}

func effectiveInputTypeParam(rule prefixRule) string {
	if strings.TrimSpace(rule.InputTypeParam) != "" {
		return rule.InputTypeParam
	}
	if strings.TrimSpace(activeConfig.InputTypeParam) != "" {
		return strings.TrimSpace(activeConfig.InputTypeParam)
	}
	return "input_type"
}

func readExtraParam(params *schemas.EmbeddingParameters, key string) string {
	if params == nil || key == "" || params.ExtraParams == nil {
		return ""
	}
	value, ok := params.ExtraParams[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	return role
}

func applyPrefix(rule prefixRule, input *schemas.EmbeddingInput, prefix string) bool {
	if input == nil {
		return false
	}

	skipIfPrefixed := shouldSkipIfPrefixed(rule)
	changed := false

	if input.Text != nil {
		next, didChange := prefixedValue(*input.Text, prefix, skipIfPrefixed)
		if didChange {
			*input.Text = next
			changed = true
		}
	}

	if len(input.Texts) > 0 {
		for i := range input.Texts {
			next, didChange := prefixedValue(input.Texts[i], prefix, skipIfPrefixed)
			if didChange {
				input.Texts[i] = next
				changed = true
			}
		}
	}

	return changed
}

func prefixedValue(value, prefix string, skipIfPrefixed bool) (string, bool) {
	if skipIfPrefixed && strings.HasPrefix(value, prefix) {
		return value, false
	}
	return prefix + value, true
}

func shouldSkipIfPrefixed(rule prefixRule) bool {
	if rule.SkipIfPrefixed != nil {
		return *rule.SkipIfPrefixed
	}
	if activeConfig.SkipIfPrefixed != nil {
		return *activeConfig.SkipIfPrefixed
	}
	return true
}

func shouldStripInputType(rule prefixRule) bool {
	if rule.StripInputType != nil {
		return *rule.StripInputType
	}
	if activeConfig.StripInputType != nil {
		return *activeConfig.StripInputType
	}
	return false
}

func stripExtraParam(req *schemas.BifrostEmbeddingRequest, key string) {
	if req == nil || req.Params == nil || req.Params.ExtraParams == nil {
		return
	}
	delete(req.Params.ExtraParams, key)
	if len(req.Params.ExtraParams) == 0 {
		req.Params.ExtraParams = nil
	}
}

func readInputTypeFromRawBody(rawBody []byte, key string) string {
	if len(rawBody) == 0 || key == "" {
		return ""
	}

	var body struct {
		ExtraParams map[string]any `json:"extra_params"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return ""
	}

	var root map[string]any
	if err := json.Unmarshal(rawBody, &root); err != nil {
		return ""
	}
	if value, ok := root[key]; ok {
		if stringValue, ok := value.(string); ok {
			return stringValue
		}
	}
	if body.ExtraParams != nil {
		if value, ok := body.ExtraParams[key]; ok {
			if stringValue, ok := value.(string); ok {
				return stringValue
			}
		}
	}
	return ""
}

func logf(ctx *schemas.BifrostContext, level schemas.LogLevel, format string, args ...any) {
	if ctx == nil {
		return
	}
	ctx.Log(level, fmt.Sprintf(format, args...))
}
