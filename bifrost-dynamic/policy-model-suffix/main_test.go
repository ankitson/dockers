package main

import (
	"reflect"
	"testing"
)

func TestParseModelPolicySuffix(t *testing.T) {
	base, policy, ok, err := parseModelPolicySuffix("deepseek/deepseek-v4-flash[zdr,provider=digitalocean]")
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
		"zdr":             true,
		"data_collection": "deny",
		"only":            []string{"digitalocean"},
		"allow_fallbacks": false,
	}
	if !reflect.DeepEqual(policy, want) {
		t.Fatalf("policy = %#v, want %#v", policy, want)
	}
}

func TestParseModelPolicySuffixSupportsSemicolonAndOverride(t *testing.T) {
	_, policy, ok, err := parseModelPolicySuffix("m[zdr;provider=digitalocean;allow_fallbacks=true]")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected suffix to parse")
	}
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
