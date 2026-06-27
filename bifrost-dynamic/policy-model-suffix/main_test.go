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
