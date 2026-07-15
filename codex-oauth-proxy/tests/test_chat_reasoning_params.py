# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Unit coverage for Chat Completions reasoning effort normalization."""

import json
import os
import subprocess
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


NODE_TEST = r"""
import assert from "node:assert/strict";
import { handleChatCompletions } from "./proxy-server.mjs";

function sseResponse() {
	const payload = [
		"data: " + JSON.stringify({
			type: "response.output_item.done",
			output_index: 0,
			item: { type: "message", role: "assistant", content: [{ type: "output_text", text: "ok" }] },
		}),
		"data: " + JSON.stringify({
			type: "response.completed",
			response: {
				id: "resp_test",
				status: "completed",
				model: "gpt-5.4",
				usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 },
			},
		}),
		"",
	].join("\n\n");
	return {
		ok: true,
		status: 200,
		headers: { get: () => "text/event-stream" },
		body: new ReadableStream({
			start(controller) {
				controller.enqueue(new TextEncoder().encode(payload));
				controller.close();
			},
		}),
	};
}

async function capturedUpstreamBody(body) {
	let captured;
	const client = {
		async request(path, init) {
			assert.equal(path, "/responses");
			captured = JSON.parse(init.body);
			return sseResponse();
		},
	};
	await handleChatCompletions({
		model: "gpt-5.4",
		messages: [{ role: "user", content: "hello" }],
		stream: false,
		...body,
	}, client, { requestId: "req_test" });
	return captured;
}

const cases = [
	{
		name: "flat-only",
		body: { reasoning_effort: "high" },
		expected: "high",
	},
	{
		name: "nested-only",
		body: { reasoning: { effort: "medium" } },
		expected: "medium",
	},
	{
		name: "both-present",
		body: { reasoning_effort: "low", reasoning: { effort: "high" } },
		expected: "low",
	},
	{
		name: "neither-present",
		body: {},
		expected: undefined,
	},
];

for (const testCase of cases) {
	const upstream = await capturedUpstreamBody(testCase.body);
	if (testCase.expected === undefined) {
		assert.equal(upstream.reasoning, undefined, testCase.name);
		assert.equal(upstream.include, undefined, testCase.name);
	} else {
		assert.deepEqual(upstream.reasoning, { effort: testCase.expected, summary: "auto" }, testCase.name);
		assert.deepEqual(upstream.include, ["reasoning.encrypted_content"], testCase.name);
	}
}
"""


class ChatReasoningParamsTest(unittest.TestCase):
	def test_chat_completions_accepts_flat_and_nested_reasoning_effort(self):
		env = os.environ | {
			"CODEX_OAUTH_PROXY_TEST": "1",
			"PROXY_LOG_LEVEL": "error",
		}
		result = subprocess.run(
			["node", "--input-type=module", "-e", NODE_TEST],
			cwd=ROOT,
			env=env,
			text=True,
			capture_output=True,
		)
		self.assertEqual(
			result.returncode,
			0,
			msg="\n".join(
				part
				for part in [
					"Node reasoning test failed.",
					"STDOUT:",
					result.stdout,
					"STDERR:",
					result.stderr,
				]
				if part
			),
		)


if __name__ == "__main__":
	unittest.main()
