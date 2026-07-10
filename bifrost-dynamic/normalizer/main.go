package main

import (
	"encoding/json"

	"github.com/maximhq/bifrost/core/schemas"
)

// providers this plugin normalizes reasoning for. Populated from the plugin
// config's `providers` array at Init; defaults to {"unsloth"} when unset so
// existing behavior is preserved. Adding a provider is a config edit, not a
// rebuild.
var normalizeProviders = map[string]bool{"unsloth": true}

type normalizerConfig struct {
	Providers []string `json:"providers"`
}

func Init(config any) error {
	if config == nil {
		return nil
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil // keep default on malformed config
	}
	var cfg normalizerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil || len(cfg.Providers) == 0 {
		return nil
	}
	set := make(map[string]bool, len(cfg.Providers))
	for _, p := range cfg.Providers {
		set[p] = true
	}
	normalizeProviders = set
	return nil
}

func GetName() string { return "normalizer" }

func Cleanup() error { return nil }

func PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

func PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

func PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if resp == nil || resp.ChatResponse == nil {
		return resp, bifrostErr, nil
	}
	provider := resp.ChatResponse.ExtraFields.Provider
	if !normalizeProviders[string(provider)] {
		return resp, bifrostErr, nil
	}

	for i := range resp.ChatResponse.Choices {
		choice := &resp.ChatResponse.Choices[i]
		if choice.ChatNonStreamResponseChoice == nil || choice.ChatNonStreamResponseChoice.Message == nil {
			continue
		}
		msg := choice.ChatNonStreamResponseChoice.Message
		if msg.ChatAssistantMessage == nil {
			continue
		}
		a := msg.ChatAssistantMessage
		if a.Reasoning != nil && a.ReasoningContent == nil {
			v := *a.Reasoning
			a.ReasoningContent = &v
		}
	}

	return resp, bifrostErr, nil
}

// HTTPTransportStreamChunkHook copies `reasoning` into `reasoning_content`
// for streaming deltas from providers that use the non-standard field name.
func HTTPTransportStreamChunkHook(_ *schemas.BifrostContext, _ *schemas.HTTPRequest, chunk *schemas.BifrostStreamChunk) (*schemas.BifrostStreamChunk, error) {
	if chunk == nil || chunk.BifrostChatResponse == nil {
		return chunk, nil
	}

	provider := chunk.BifrostChatResponse.ExtraFields.Provider
	if !normalizeProviders[string(provider)] {
		return chunk, nil
	}

	for i := range chunk.BifrostChatResponse.Choices {
		choice := &chunk.BifrostChatResponse.Choices[i]
		if choice.ChatStreamResponseChoice == nil || choice.ChatStreamResponseChoice.Delta == nil {
			continue
		}
		delta := choice.ChatStreamResponseChoice.Delta
		if delta.Reasoning != nil && delta.ReasoningContent == nil {
			v := *delta.Reasoning
			delta.ReasoningContent = &v
		}
	}

	return chunk, nil
}
