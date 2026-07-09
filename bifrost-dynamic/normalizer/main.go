package main

import "github.com/maximhq/bifrost/core/schemas"

func Init(config any) error { return nil }

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
	if provider != "unsloth" {
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
	if provider != "unsloth" {
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
