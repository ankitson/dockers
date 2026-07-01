package main

import (
	"reflect"
	"testing"
)

func TestParseModelPolicySuffix(t *testing.T) {
	base, extra, ok, err := parseModelPolicySuffix("deepseek/deepseek-v4-flash[zdr,provider=digitalocean]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
	if base != "deepseek/deepseek-v4-flash" {
		t.Fatalf("base = %q", base)
	}

	want := map[string]any{
		"provider": map[string]any{
			"zdr":             true,
			"data_collection": "deny",
			"only":            []string{"digitalocean"},
			"allow_fallbacks": false,
		},
	}
	if !reflect.DeepEqual(extra, want) {
		t.Fatalf("extra = %#v, want %#v", extra, want)
	}
}

func TestParseModelPolicySuffixSupportsSemicolonAndOverride(t *testing.T) {
	_, extra, ok, err := parseModelPolicySuffix("m[zdr;provider=digitalocean;allow_fallbacks=true]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
	policy := extra["provider"].(map[string]any)
	if policy["allow_fallbacks"] != true {
		t.Fatalf("allow_fallbacks = %#v", policy["allow_fallbacks"])
	}
}

func TestParseModelPolicySuffixNoSuffix(t *testing.T) {
	base, policy, ok, err := parseModelPolicySuffix("deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	if ok || policy != nil || base != "deepseek/deepseek-v4-flash" {
		t.Fatalf("base=%q policy=%#v ok=%v", base, policy, ok)
	}
}

func TestParseModelPolicySuffixJSONBody(t *testing.T) {
	base, extra, ok, err := parseModelPolicySuffix(`deepseek/deepseek-v4-flash[{"provider":{"zdr":true,"only":["digitalocean"],"allow_fallbacks":false},"reasoning":{"effort":"high"},"max_tokens":64}]`)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
	if base != "deepseek/deepseek-v4-flash" {
		t.Fatalf("base = %q", base)
	}

	want := map[string]any{
		"provider": map[string]any{
			"zdr":             true,
			"only":            []any{"digitalocean"},
			"allow_fallbacks": false,
		},
		"reasoning":  map[string]any{"effort": "high"},
		"max_tokens": float64(64),
	}
	if !reflect.DeepEqual(extra, want) {
		t.Fatalf("extra = %#v, want %#v", extra, want)
	}
}

func TestParseModelPolicySuffixQuotedJSONBody(t *testing.T) {
	_, extra, ok, err := parseModelPolicySuffix(`m["{\"provider\":{\"only\":[\"digitalocean\"]},\"usage\":{\"include\":true}}"]`)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}

	want := map[string]any{
		"provider": map[string]any{"only": []any{"digitalocean"}},
		"usage":    map[string]any{"include": true},
	}
	if !reflect.DeepEqual(extra, want) {
		t.Fatalf("extra = %#v, want %#v", extra, want)
	}
}

func TestParseModelPolicySuffixDottedDirectives(t *testing.T) {
	_, extra, ok, err := parseModelPolicySuffix("m[provider.only=digitalocean,reasoning.effort=high,max_tokens=64]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}

	want := map[string]any{
		"provider":   map[string]any{"only": []string{"digitalocean"}, "allow_fallbacks": false},
		"reasoning":  map[string]any{"effort": "high"},
		"max_tokens": int64(64),
	}
	if !reflect.DeepEqual(extra, want) {
		t.Fatalf("extra = %#v, want %#v", extra, want)
	}
}

func TestParseModelPolicySuffixQueryBody(t *testing.T) {
	_, extra, ok, err := parseModelPolicySuffix("m[?provider.only=digitalocean&provider.allow_fallbacks=false&reasoning.effort=high]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}

	want := map[string]any{
		"provider":  map[string]any{"only": []string{"digitalocean"}, "allow_fallbacks": false},
		"reasoning": map[string]any{"effort": "high"},
	}
	if !reflect.DeepEqual(extra, want) {
		t.Fatalf("extra = %#v, want %#v", extra, want)
	}
}

func TestOpenRouterTEEAliasPinsPhalaZDR(t *testing.T) {
	base, extra, ok, err := parseModelPolicySuffix("openai/gpt-oss-20b[tee]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
	model, extra := applyPrivacyAliases(nil, "openrouter", base, extra)
	if model != "openai/gpt-oss-20b" {
		t.Fatalf("model = %q", model)
	}

	want := map[string]any{
		"provider": map[string]any{
			"only":            []string{"phala"},
			"allow_fallbacks": false,
			"zdr":             true,
			"data_collection": "deny",
		},
	}
	if !reflect.DeepEqual(extra, want) {
		t.Fatalf("extra = %#v, want %#v", extra, want)
	}
}

func TestOpenRouterTEEAliasKeepsExplicitProvider(t *testing.T) {
	base, extra, ok, err := parseModelPolicySuffix("m[tee,provider=chutes]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
	_, extra = applyPrivacyAliases(nil, "openrouter", base, extra)
	policy := extra["provider"].(map[string]any)
	if !reflect.DeepEqual(policy["only"], []string{"chutes"}) {
		t.Fatalf("only = %#v", policy["only"])
	}
	if policy["allow_fallbacks"] != false {
		t.Fatalf("allow_fallbacks = %#v", policy["allow_fallbacks"])
	}
}

func TestVeniceE2EEAliasMapsFriendlyModel(t *testing.T) {
	base, extra, ok, err := parseModelPolicySuffix("gpt-oss-20b[e2ee]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
	model, extra := applyPrivacyAliases(nil, "venice", base, extra)
	if model != "e2ee-gpt-oss-20b-p" {
		t.Fatalf("model = %q", model)
	}
	if len(extra) != 0 {
		t.Fatalf("extra = %#v, want empty", extra)
	}
}

func TestVeniceTEEAliasLeavesExplicitTEEModel(t *testing.T) {
	base, extra, ok, err := parseModelPolicySuffix("tee-qwen3-5-122b-a10b[tee]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
	model, extra := applyPrivacyAliases(nil, "venice", base, extra)
	if model != "tee-qwen3-5-122b-a10b" {
		t.Fatalf("model = %q", model)
	}
	if len(extra) != 0 {
		t.Fatalf("extra = %#v, want empty", extra)
	}
}
