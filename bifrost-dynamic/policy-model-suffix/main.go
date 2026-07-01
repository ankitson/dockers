package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

func Init(config any) error { return nil }

func GetName() string { return "model-policy-suffix" }

func Cleanup() error { return nil }

func PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	provider, model, _ := req.GetRequestFields()
	if model == "" {
		return nil
	}

	baseModel, extraParams, ok, err := parseModelPolicySuffix(model)
	if err != nil {
		ctx.Log(schemas.LogLevelWarn, fmt.Sprintf("model-policy-suffix ignored malformed suffix on %q: %v", model, err))
		return nil
	}
	if !ok {
		return nil
	}

	baseModel, extraParams = applyPrivacyAliases(ctx, provider, baseModel, extraParams)
	req.SetModel(baseModel)
	if len(extraParams) > 0 {
		if !mergeExtraParams(req, extraParams) {
			ctx.Log(schemas.LogLevelWarn, fmt.Sprintf("model-policy-suffix could not attach extra params to request type %s", req.RequestType))
			return nil
		}
		ctx.SetValue(schemas.BifrostContextKeyPassthroughExtraParams, true)
	}

	ctx.Log(schemas.LogLevelInfo, fmt.Sprintf("model-policy-suffix applied policy to %s/%q -> %q", provider, model, baseModel))
	return nil
}

func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

func parseModelPolicySuffix(model string) (string, map[string]any, bool, error) {
	if !strings.HasSuffix(model, "]") {
		return model, nil, false, nil
	}

	open, ok, err := findSuffixOpen(model)
	if err != nil {
		return model, nil, false, err
	}
	if !ok {
		return model, nil, false, fmt.Errorf("missing opening bracket")
	}

	base := strings.TrimSpace(model[:open])
	body := strings.TrimSpace(model[open+1 : len(model)-1])
	if base == "" || body == "" {
		return model, nil, false, fmt.Errorf("empty base model or policy")
	}

	extraParams, err := parseSuffixBody(body)
	if err != nil {
		return model, nil, false, err
	}

	return base, extraParams, true, nil
}

func findSuffixOpen(model string) (int, bool, error) {
	depth := 0
	start := -1
	inString := false
	escaped := false

	for i := 0; i < len(model); i++ {
		c := model[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '[':
			if depth == 0 {
				start = i
			}
			depth++
		case ']':
			if depth == 0 {
				return -1, false, fmt.Errorf("unmatched closing bracket")
			}
			depth--
			if depth == 0 && i == len(model)-1 {
				return start, true, nil
			}
		}
	}

	return -1, false, nil
}

func parseSuffixBody(body string) (map[string]any, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("empty policy")
	}

	if strings.HasPrefix(body, "{") {
		return parseJSONObject(body)
	}
	if strings.HasPrefix(body, "?") {
		return parseQueryBody(strings.TrimPrefix(body, "?"))
	}
	if strings.HasPrefix(body, "json64:") {
		return parseBase64JSONObject(strings.TrimPrefix(body, "json64:"))
	}
	if strings.HasPrefix(body, "json64=") {
		return parseBase64JSONObject(strings.TrimPrefix(body, "json64="))
	}
	if strings.HasPrefix(body, "\"") {
		var decoded string
		if err := json.Unmarshal([]byte(body), &decoded); err != nil {
			return nil, fmt.Errorf("invalid quoted policy: %w", err)
		}
		return parseSuffixBody(decoded)
	}

	return parseDirectiveBody(body)
}

func parseBase64JSONObject(encoded string) (map[string]any, error) {
	parsed, err := parseBase64JSON(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 JSON policy: %w", err)
	}
	extra, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("base64 JSON policy must decode to an object")
	}
	return extra, nil
}

func parseJSONObject(body string) (map[string]any, error) {
	var extra map[string]any
	if err := json.Unmarshal([]byte(body), &extra); err != nil {
		return nil, fmt.Errorf("invalid JSON policy: %w", err)
	}
	return extra, nil
}

func parseDirectiveBody(body string) (map[string]any, error) {
	extraParams := map[string]any{}
	providerPolicy := map[string]any{}
	privacyPolicy := map[string]any{}
	pinnedProvider := false
	fallbackExplicit := false

	for _, part := range splitDirectives(body) {
		if part == "" {
			continue
		}
		key, value, hasValue := strings.Cut(part, "=")
		key = normalizeKey(key, true)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("empty directive in %q", part)
		}

		if !hasValue {
			switch key {
			case "tee":
				privacyPolicy["tee"] = true
			case "zdr":
				providerPolicy["zdr"] = true
				providerPolicy["data_collection"] = "deny"
			case "e2ee":
				privacyPolicy["e2ee"] = true
			case "no_fallbacks", "no-fallbacks":
				providerPolicy["allow_fallbacks"] = false
				fallbackExplicit = true
			default:
				return nil, fmt.Errorf("unknown bare directive %q", key)
			}
			continue
		}

		switch key {
		case "tee":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid tee value %q", value)
			}
			privacyPolicy["tee"] = b
		case "zdr":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid zdr value %q", value)
			}
			providerPolicy["zdr"] = b
			if b {
				providerPolicy["data_collection"] = "deny"
			}
		case "e2ee":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid e2ee value %q", value)
			}
			privacyPolicy["e2ee"] = b
		case "privacy":
			parsed, err := parseValue(value)
			if err != nil {
				return nil, fmt.Errorf("invalid privacy value: %w", err)
			}
			if privacyMap, ok := parsed.(map[string]any); ok {
				mergeMaps(privacyPolicy, privacyMap)
			} else {
				privacyPolicy[fmt.Sprint(parsed)] = true
			}
		case "provider":
			parsed, err := parseValue(value)
			if err != nil {
				return nil, fmt.Errorf("invalid provider value: %w", err)
			}
			if providerMap, ok := parsed.(map[string]any); ok {
				mergeMaps(providerPolicy, providerMap)
				break
			}
			providerPolicy["only"] = []string{fmt.Sprint(parsed)}
			pinnedProvider = true
		case "only", "providers":
			providerPolicy["only"] = splitListValue(value)
			pinnedProvider = true
		case "order", "ignore":
			providerPolicy[key] = splitListValue(value)
		case "fallbacks", "allow_fallbacks", "allow-fallbacks":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid allow_fallbacks value %q", value)
			}
			providerPolicy["allow_fallbacks"] = b
			fallbackExplicit = true
		default:
			parsed, err := parseValue(value)
			if err != nil {
				return nil, fmt.Errorf("invalid value for %q: %w", key, err)
			}
			if strings.HasPrefix(key, "provider.") {
				providerKey := normalizeKey(strings.TrimPrefix(key, "provider."), false)
				providerPolicy[providerKey] = normalizeProviderValue(providerKey, parsed)
				continue
			}
			if isProviderPreferenceKey(key) {
				providerPolicy[key] = normalizeProviderValue(key, parsed)
				continue
			}
			setNested(extraParams, strings.Split(key, "."), parsed)
		}
	}

	if pinnedProvider && !fallbackExplicit {
		providerPolicy["allow_fallbacks"] = false
	}
	if len(providerPolicy) > 0 {
		extraParams["provider"] = providerPolicy
	}
	if len(privacyPolicy) > 0 {
		extraParams["_privacy"] = privacyPolicy
	}

	return extraParams, nil
}

func parseQueryBody(body string) (map[string]any, error) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, fmt.Errorf("invalid query policy: %w", err)
	}

	extraParams := map[string]any{}
	providerPolicy := map[string]any{}
	privacyPolicy := map[string]any{}
	pinnedProvider := false
	fallbackExplicit := false
	for key, rawValues := range values {
		key = normalizeKey(key, false)
		var value any
		if len(rawValues) == 1 {
			parsed, err := parseValue(rawValues[0])
			if err != nil {
				return nil, fmt.Errorf("invalid value for %q: %w", key, err)
			}
			value = parsed
		} else {
			items := make([]any, 0, len(rawValues))
			for _, raw := range rawValues {
				parsed, err := parseValue(raw)
				if err != nil {
					return nil, fmt.Errorf("invalid value for %q: %w", key, err)
				}
				items = append(items, parsed)
			}
			value = items
		}

		if strings.HasPrefix(key, "provider.") {
			providerKey := normalizeKey(strings.TrimPrefix(key, "provider."), false)
			providerPolicy[providerKey] = normalizeProviderValue(providerKey, value)
			if providerKey == "only" {
				pinnedProvider = true
			}
			if providerKey == "allow_fallbacks" {
				fallbackExplicit = true
			}
			continue
		}
		if strings.HasPrefix(key, "privacy.") {
			privacyKey := normalizeKey(strings.TrimPrefix(key, "privacy."), false)
			privacyPolicy[privacyKey] = value
			continue
		}
		switch key {
		case "tee", "e2ee":
			privacyPolicy[key] = value
			continue
		case "privacy":
			if privacyMap, ok := value.(map[string]any); ok {
				mergeMaps(privacyPolicy, privacyMap)
			} else {
				privacyPolicy[fmt.Sprint(value)] = true
			}
			continue
		}
		if isProviderPreferenceKey(key) {
			providerPolicy[key] = normalizeProviderValue(key, value)
			if key == "only" {
				pinnedProvider = true
			}
			if key == "allow_fallbacks" {
				fallbackExplicit = true
			}
			continue
		}
		setNested(extraParams, strings.Split(key, "."), value)
	}
	if pinnedProvider && !fallbackExplicit {
		providerPolicy["allow_fallbacks"] = false
	}
	if len(providerPolicy) > 0 {
		extraParams["provider"] = providerPolicy
	}
	if len(privacyPolicy) > 0 {
		extraParams["_privacy"] = privacyPolicy
	}

	return extraParams, nil
}

func applyPrivacyAliases(ctx *schemas.BifrostContext, provider schemas.ModelProvider, model string, extraParams map[string]any) (string, map[string]any) {
	privacy, ok := extraParams["_privacy"].(map[string]any)
	if !ok {
		return model, extraParams
	}
	delete(extraParams, "_privacy")

	tee := truthy(privacy["tee"])
	e2ee := truthy(privacy["e2ee"])
	switch strings.ToLower(string(provider)) {
	case string(schemas.OpenRouter):
		if tee {
			providerPolicy := ensureProviderPolicy(extraParams)
			if _, hasOnly := providerPolicy["only"]; !hasOnly {
				providerPolicy["only"] = []string{"phala"}
			}
			providerPolicy["allow_fallbacks"] = false
			providerPolicy["zdr"] = true
			providerPolicy["data_collection"] = "deny"
		}
		if e2ee {
			logWarn(ctx, "model-policy-suffix [e2ee] requested for OpenRouter, but OpenRouter routing cannot add client-side encryption")
		}
	case "venice":
		if e2ee {
			model = veniceE2EEModel(model)
			logWarn(ctx, "model-policy-suffix [e2ee] selected a Venice E2EE model, but client-side encryption and Venice E2EE headers are still required")
		} else if tee {
			model = veniceTEEModel(model)
		}
	default:
		if tee || e2ee {
			logWarn(ctx, fmt.Sprintf("model-policy-suffix privacy alias ignored for unsupported provider %s", provider))
		}
	}
	return model, extraParams
}

func logWarn(ctx *schemas.BifrostContext, message string) {
	if ctx != nil {
		ctx.Log(schemas.LogLevelWarn, message)
	}
}

func ensureProviderPolicy(extraParams map[string]any) map[string]any {
	providerPolicy, _ := extraParams["provider"].(map[string]any)
	if providerPolicy == nil {
		providerPolicy = map[string]any{}
		extraParams["provider"] = providerPolicy
	}
	return providerPolicy
}

func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		b, err := strconv.ParseBool(v)
		return err == nil && b
	default:
		return false
	}
}

func veniceTEEModel(model string) string {
	if strings.HasPrefix(model, "tee-") || strings.HasPrefix(model, "e2ee-") {
		return model
	}
	return veniceE2EEModel(model)
}

func veniceE2EEModel(model string) string {
	if strings.HasPrefix(model, "e2ee-") {
		return model
	}
	aliases := map[string]string{
		"gpt-oss-20b":                "e2ee-gpt-oss-20b-p",
		"openai-gpt-oss-20b":         "e2ee-gpt-oss-20b-p",
		"gpt-oss-120b":               "e2ee-gpt-oss-120b-p",
		"openai-gpt-oss-120b":        "e2ee-gpt-oss-120b-p",
		"gemma-4-31b":                "e2ee-gemma-4-31b",
		"google-gemma-4-31b-it":      "e2ee-gemma-4-31b",
		"gemma-3-27b":                "e2ee-gemma-3-27b-p",
		"gemma-4-26b-a4b-uncensored": "e2ee-gemma-4-26b-a4b-uncensored-p",
		"google-gemma-4-26b-a4b-it":  "e2ee-gemma-4-26b-a4b-uncensored-p",
		"glm-5-2":                    "e2ee-glm-5-2-p",
		"zai-org-glm-5-2":            "e2ee-glm-5-2-p",
		"glm-5-1":                    "e2ee-glm-5-1",
		"zai-org-glm-5-1":            "e2ee-glm-5-1",
		"glm-4-7":                    "e2ee-glm-4-7-p",
		"zai-org-glm-4-7":            "e2ee-glm-4-7-p",
		"qwen-2-5-7b":                "e2ee-qwen-2-5-7b-p",
		"qwen3-30b-a3b":              "e2ee-qwen3-30b-a3b-p",
		"qwen3-6-35b-a3b":            "e2ee-qwen3-6-35b-a3b",
		"qwen3-6-35b-a3b-uncensored": "e2ee-qwen3-6-35b-a3b-uncensored-p",
		"qwen3-vl-30b-a3b":           "e2ee-qwen3-vl-30b-a3b-p",
		"venice-uncensored-24b":      "e2ee-venice-uncensored-24b-p",
		"venice-uncensored-1-1":      "e2ee-venice-uncensored-24b-p",
		"e2ee-venice-uncensored-1-1": "e2ee-venice-uncensored-24b-p",
		"e2ee-venice-uncensored-24b": "e2ee-venice-uncensored-24b-p",
	}
	if resolved, ok := aliases[model]; ok {
		return resolved
	}
	return "e2ee-" + model
}

func splitDirectives(body string) []string {
	return splitTopLevel(body, ",;")
}

func splitTopLevel(body string, separators string) []string {
	var out []string
	start := 0
	objectDepth := 0
	arrayDepth := 0
	inString := false
	escaped := false

	for i := 0; i < len(body); i++ {
		c := body[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{':
			objectDepth++
		case '}':
			if objectDepth > 0 {
				objectDepth--
			}
		case '[':
			arrayDepth++
		case ']':
			if arrayDepth > 0 {
				arrayDepth--
			}
		default:
			if objectDepth == 0 && arrayDepth == 0 && strings.ContainsRune(separators, rune(c)) {
				out = append(out, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}

	out = append(out, strings.TrimSpace(body[start:]))
	return out
}

func normalizeKey(key string, trimProviderPrefix bool) string {
	key = strings.TrimSpace(strings.ToLower(key))
	if trimProviderPrefix {
		switch key {
		case "provider.only", "provider.providers":
			key = "only"
		case "provider.order", "provider.ignore", "provider.zdr", "provider.data_collection", "provider.allow_fallbacks", "provider.allow-fallbacks":
			key = strings.TrimPrefix(key, "provider.")
		}
	}
	return strings.ReplaceAll(key, "-", "_")
}

func splitListValue(value string) []string {
	raw := strings.FieldsFunc(value, func(r rune) bool {
		return r == '|' || r == '+'
	})
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func parseValue(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") || strings.HasPrefix(value, "\"") {
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	}
	if b, err := strconv.ParseBool(value); err == nil {
		return b, nil
	}
	if value == "null" {
		return nil, nil
	}
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f, nil
	}
	if strings.HasPrefix(value, "json64:") {
		return parseBase64JSON(strings.TrimPrefix(value, "json64:"))
	}
	if strings.HasPrefix(value, "json64=") {
		return parseBase64JSON(strings.TrimPrefix(value, "json64="))
	}
	return value, nil
}

func parseBase64JSON(encoded string) (any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, err
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func isProviderPreferenceKey(key string) bool {
	switch key {
	case "zdr", "data_collection", "enforce_distillable_text", "order", "allow_fallbacks", "require_parameters",
		"only", "ignore", "quantizations", "sort", "preferred_min_throughput", "preferred_max_latency", "max_price":
		return true
	default:
		return false
	}
}

func normalizeProviderValue(key string, value any) any {
	switch key {
	case "only", "order", "ignore", "quantizations":
		if s, ok := value.(string); ok {
			return splitListValue(s)
		}
	}
	return value
}

func setNested(target map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		target[path[0]] = value
		return
	}

	key := path[0]
	child, _ := target[key].(map[string]any)
	if child == nil {
		child = map[string]any{}
		target[key] = child
	}
	setNested(child, path[1:], value)
}

func mergeExtraParams(req *schemas.BifrostRequest, params map[string]any) bool {
	extra := requestExtraParams(req)
	if extra == nil {
		return false
	}

	mergeMaps(extra, params)
	return true
}

func mergeMaps(dst map[string]any, src map[string]any) {
	for key, value := range src {
		srcMap, srcIsMap := value.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap && dstIsMap {
			mergeMaps(dstMap, srcMap)
			continue
		}
		dst[key] = value
	}
}

func requestExtraParams(req *schemas.BifrostRequest) map[string]any {
	switch {
	case req.ChatRequest != nil:
		if req.ChatRequest.Params == nil {
			req.ChatRequest.Params = &schemas.ChatParameters{}
		}
		if req.ChatRequest.Params.ExtraParams == nil {
			req.ChatRequest.Params.ExtraParams = map[string]any{}
		}
		return req.ChatRequest.Params.ExtraParams
	case req.TextCompletionRequest != nil:
		if req.TextCompletionRequest.Params == nil {
			req.TextCompletionRequest.Params = &schemas.TextCompletionParameters{}
		}
		if req.TextCompletionRequest.Params.ExtraParams == nil {
			req.TextCompletionRequest.Params.ExtraParams = map[string]any{}
		}
		return req.TextCompletionRequest.Params.ExtraParams
	case req.ResponsesRequest != nil:
		if req.ResponsesRequest.Params == nil {
			req.ResponsesRequest.Params = &schemas.ResponsesParameters{}
		}
		if req.ResponsesRequest.Params.ExtraParams == nil {
			req.ResponsesRequest.Params.ExtraParams = map[string]any{}
		}
		return req.ResponsesRequest.Params.ExtraParams
	default:
		return nil
	}
}
