package main

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestInitNormalizesPrefixKeys(t *testing.T) {
	err := Init(map[string]any{
		"rules": []map[string]any{
			{
				"provider":     "ollama",
				"model_prefix": "nomic-embed-text",
				"prefixes": map[string]any{
					"Query":    "search_query: ",
					"document": "search_document: ",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	rule, ok := findMatchingRule("ollama", "nomic-embed-text:latest")
	if !ok {
		t.Fatal("expected rule match")
	}
	if rule.Prefixes["query"] != "search_query: " {
		t.Fatalf("query prefix = %q", rule.Prefixes["query"])
	}
}

func TestPreRequestHookPrefixesQueryInput(t *testing.T) {
	loadTestConfig(t, map[string]any{
		"strip_input_type": true,
		"rules": []map[string]any{
			{
				"provider":     "ollama",
				"model_prefix": "nomic-embed-text",
				"prefixes": map[string]any{
					"query":    "search_query: ",
					"document": "search_document: ",
				},
			},
		},
	})

	input := "what is bifrost"
	req := &schemas.BifrostRequest{
		RequestType: schemas.EmbeddingRequest,
		EmbeddingRequest: &schemas.BifrostEmbeddingRequest{
			Provider: "ollama",
			Model:    "nomic-embed-text:latest",
			Input:    &schemas.EmbeddingInput{Text: &input},
			Params: &schemas.EmbeddingParameters{
				ExtraParams: map[string]any{"input_type": "query"},
			},
			RawRequestBody: []byte(`{"model":"ollama/nomic-embed-text:latest","input":"what is bifrost","input_type":"query"}`),
		},
	}

	if err := PreRequestHook(nil, req); err != nil {
		t.Fatal(err)
	}

	if got := *req.EmbeddingRequest.Input.Text; got != "search_query: what is bifrost" {
		t.Fatalf("text = %q", got)
	}
	if req.EmbeddingRequest.Params.ExtraParams != nil {
		t.Fatalf("extra params = %#v, want nil after stripping", req.EmbeddingRequest.Params.ExtraParams)
	}
}

func TestPreRequestHookPrefixesDocumentBatch(t *testing.T) {
	loadTestConfig(t, map[string]any{
		"rules": []map[string]any{
			{
				"provider": "ollama",
				"model":    "nomic-embed-text:latest",
				"prefixes": map[string]any{
					"document": "search_document: ",
				},
			},
		},
	})

	req := &schemas.BifrostRequest{
		RequestType: schemas.EmbeddingRequest,
		EmbeddingRequest: &schemas.BifrostEmbeddingRequest{
			Provider: "ollama",
			Model:    "nomic-embed-text:latest",
			Input: &schemas.EmbeddingInput{
				Texts: []string{"doc one", "doc two"},
			},
			Params: &schemas.EmbeddingParameters{
				ExtraParams: map[string]any{"input_type": "document"},
			},
			RawRequestBody: []byte(`{"model":"ollama/nomic-embed-text:latest","input":["doc one","doc two"],"input_type":"document"}`),
		},
	}

	if err := PreRequestHook(nil, req); err != nil {
		t.Fatal(err)
	}

	got := req.EmbeddingRequest.Input.Texts
	if got[0] != "search_document: doc one" || got[1] != "search_document: doc two" {
		t.Fatalf("texts = %#v", got)
	}
}

func TestPreRequestHookSkipsAlreadyPrefixedInput(t *testing.T) {
	loadTestConfig(t, map[string]any{
		"rules": []map[string]any{
			{
				"provider":     "ollama",
				"model_prefix": "nomic-embed-text",
				"prefixes": map[string]any{
					"query": "search_query: ",
				},
			},
		},
	})

	input := "search_query: already normalized"
	req := &schemas.BifrostRequest{
		RequestType: schemas.EmbeddingRequest,
		EmbeddingRequest: &schemas.BifrostEmbeddingRequest{
			Provider: "ollama",
			Model:    "nomic-embed-text:latest",
			Input:    &schemas.EmbeddingInput{Text: &input},
			Params: &schemas.EmbeddingParameters{
				ExtraParams: map[string]any{"input_type": "query"},
			},
			RawRequestBody: []byte(`{"model":"ollama/nomic-embed-text:latest","input":"search_query: already normalized","input_type":"query"}`),
		},
	}

	if err := PreRequestHook(nil, req); err != nil {
		t.Fatal(err)
	}

	if got := *req.EmbeddingRequest.Input.Text; got != input {
		t.Fatalf("text = %q", got)
	}
}

func TestPreRequestHookLeavesUnmatchedRequestsAlone(t *testing.T) {
	loadTestConfig(t, map[string]any{
		"rules": []map[string]any{
			{
				"provider":     "ollama",
				"model_prefix": "nomic-embed-text",
				"prefixes": map[string]any{
					"query": "search_query: ",
				},
			},
		},
	})

	input := "raw text"
	req := &schemas.BifrostRequest{
		RequestType: schemas.EmbeddingRequest,
		EmbeddingRequest: &schemas.BifrostEmbeddingRequest{
			Provider: "openai",
			Model:    "text-embedding-3-small",
			Input:    &schemas.EmbeddingInput{Text: &input},
			Params: &schemas.EmbeddingParameters{
				ExtraParams: map[string]any{"input_type": "query"},
			},
			RawRequestBody: []byte(`{"model":"text-embedding-3-small","input":"raw text","input_type":"query"}`),
		},
	}

	if err := PreRequestHook(nil, req); err != nil {
		t.Fatal(err)
	}

	if got := *req.EmbeddingRequest.Input.Text; got != "raw text" {
		t.Fatalf("text = %q", got)
	}
}

func TestPreRequestHookReadsInputTypeFromRawBody(t *testing.T) {
	loadTestConfig(t, map[string]any{
		"rules": []map[string]any{
			{
				"provider":     "ollama",
				"model_prefix": "nomic-embed-text",
				"prefixes": map[string]any{
					"query": "search_query: ",
				},
			},
		},
	})

	input := "raw body only"
	req := &schemas.BifrostRequest{
		RequestType: schemas.EmbeddingRequest,
		EmbeddingRequest: &schemas.BifrostEmbeddingRequest{
			Provider:       "ollama",
			Model:          "nomic-embed-text:latest",
			Input:          &schemas.EmbeddingInput{Text: &input},
			RawRequestBody: []byte(`{"model":"ollama/nomic-embed-text:latest","input":"raw body only","input_type":"query"}`),
		},
	}

	if err := PreRequestHook(nil, req); err != nil {
		t.Fatal(err)
	}

	if got := *req.EmbeddingRequest.Input.Text; got != "search_query: raw body only" {
		t.Fatalf("text = %q", got)
	}
}

func loadTestConfig(t *testing.T, config map[string]any) {
	t.Helper()
	if err := Init(config); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = Cleanup()
	})
}
