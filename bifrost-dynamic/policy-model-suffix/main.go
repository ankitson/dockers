package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

func Init(config any) error { return nil }

func GetName() string { return "model-policy-suffix" }

func Cleanup() error { return nil }

func PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error {
	provider, model, _ := req.GetRequestFields()
	if provider != schemas.OpenRouter || model == "" {
		return nil
	}

	baseModel, providerPolicy, ok, err := parseModelPolicySuffix(model)
	if err != nil {
		ctx.Log(schemas.LogLevelWarn, fmt.Sprintf("model-policy-suffix ignored malformed suffix on %q: %v", model, err))
		return nil
	}
	if !ok {
		return nil
	}

	req.SetModel(baseModel)
	if len(providerPolicy) > 0 {
		if !mergeProviderPolicy(req, providerPolicy) {
			ctx.Log(schemas.LogLevelWarn, fmt.Sprintf("model-policy-suffix could not attach provider policy to request type %s", req.RequestType))
			return nil
		}
		ctx.SetValue(schemas.BifrostContextKeyPassthroughExtraParams, true)
	}

	ctx.Log(schemas.LogLevelInfo, fmt.Sprintf("model-policy-suffix applied OpenRouter policy to %q -> %q", model, baseModel))
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

	open := strings.LastIndex(model, "[")
	if open < 0 {
		return model, nil, false, fmt.Errorf("missing opening bracket")
	}

	base := strings.TrimSpace(model[:open])
	body := strings.TrimSpace(model[open+1 : len(model)-1])
	if base == "" || body == "" {
		return model, nil, false, fmt.Errorf("empty base model or policy")
	}

	providerPolicy := map[string]any{}
	pinnedProvider := false
	fallbackExplicit := false

	for _, part := range splitDirectives(body) {
		if part == "" {
			continue
		}
		key, value, hasValue := strings.Cut(part, "=")
		key = normalizeKey(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return model, nil, false, fmt.Errorf("empty directive in %q", part)
		}

		if !hasValue {
			switch key {
			case "zdr":
				providerPolicy["zdr"] = true
				providerPolicy["data_collection"] = "deny"
			case "no_fallbacks", "no-fallbacks":
				providerPolicy["allow_fallbacks"] = false
				fallbackExplicit = true
			default:
				return model, nil, false, fmt.Errorf("unknown bare directive %q", key)
			}
			continue
		}

		switch key {
		case "zdr":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return model, nil, false, fmt.Errorf("invalid zdr value %q", value)
			}
			providerPolicy["zdr"] = b
			if b {
				providerPolicy["data_collection"] = "deny"
			}
		case "provider":
			providerPolicy["only"] = []string{value}
			pinnedProvider = true
		case "only", "providers":
			providerPolicy["only"] = splitListValue(value)
			pinnedProvider = true
		case "order", "ignore":
			providerPolicy[key] = splitListValue(value)
		case "fallbacks", "allow_fallbacks", "allow-fallbacks":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return model, nil, false, fmt.Errorf("invalid allow_fallbacks value %q", value)
			}
			providerPolicy["allow_fallbacks"] = b
			fallbackExplicit = true
		default:
			providerPolicy[key] = parseScalar(value)
		}
	}

	if pinnedProvider && !fallbackExplicit {
		providerPolicy["allow_fallbacks"] = false
	}

	return base, providerPolicy, true, nil
}

func splitDirectives(body string) []string {
	return strings.FieldsFunc(body, func(r rune) bool {
		return r == ',' || r == ';'
	})
}

func normalizeKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	key = strings.TrimPrefix(key, "provider.")
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

func parseScalar(value string) any {
	if b, err := strconv.ParseBool(value); err == nil {
		return b
	}
	return value
}

func mergeProviderPolicy(req *schemas.BifrostRequest, policy map[string]any) bool {
	extra := requestExtraParams(req)
	if extra == nil {
		return false
	}

	existing, _ := extra["provider"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	for key, value := range policy {
		existing[key] = value
	}
	extra["provider"] = existing
	return true
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
