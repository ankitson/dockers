#!/usr/bin/env node
// Continuation-aware replacement for openai-oauth's built-in HTTP server.
//
// openai-oauth's handleChatCompletionsRequest calls the Vercel AI SDK's
// generateText() fresh on every single turn, fully re-translating the whole
// message history with zero reasoning-continuity metadata: on the turn after
// a tool call, the model has to re-derive its whole plan from scratch with no
// memory of its own reasoning, and occasionally produces neither text nor a
// further tool call.
//
// The obvious fix (store:true + previous_response_id, server-managed state)
// is NOT available: this specific ChatGPT/Codex backend rejects store:true
// outright ("Store must be set to false") — confirmed by direct testing, and
// matches why openai-oauth's own /v1/responses handler hard-blocks
// previous_response_id/item_reference. There is no server-side state here.
//
// So this implements the OTHER documented mechanism: stateless client-managed
// reasoning continuity. Every response is requested with
// include:["reasoning.encrypted_content"], and we cache the raw Responses API
// output items (reasoning + function_call) for every tool-calling turn, keyed
// by that turn's tool_call ids (stable and unique regardless of how a client
// reformats the surrounding JSON on the next request). When replaying history
// for a later turn, any assistant tool-call message we recognize gets its
// EXACT original items (including encrypted_content) spliced back into the
// input array, instead of being reconstructed lossily from the Chat
// Completions tool_calls shape. OAuth/base URL resolution is reused from
// openai-oauth's own exported createCodexOAuthClient.
import { execSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { createServer } from "node:http";
import { pathToFileURL } from "node:url";

const globalRoot = execSync("npm root -g").toString().trim();
const { createCodexOAuthClient, resolveOpenAIOAuthModels } = await import(
	pathToFileURL(`${globalRoot}/openai-oauth/dist/chunk-2AENSHRT.js`).href
);

const PORT = Number(process.env.PORT ?? 10531);
const HOST = process.env.HOST ?? "0.0.0.0";
const OAUTH_FILE = process.env.OAUTH_FILE ?? "/codex/auth.json";

const MAX_CACHE_ENTRIES = 2000;
const CACHE_TTL_MS = 6 * 60 * 60 * 1000; // 6h — well past any realistic agent-turn gap

/** turn key (hash of sorted tool_call ids) -> {items: RawResponsesOutputItem[], expiresAt} */
const turnCache = new Map();

function trimCache() {
	const now = Date.now();
	for (const [key, value] of turnCache) {
		if (value.expiresAt < now) turnCache.delete(key);
	}
	while (turnCache.size > MAX_CACHE_ENTRIES) {
		const oldestKey = turnCache.keys().next().value;
		turnCache.delete(oldestKey);
	}
}

function turnKey(model, toolCallIds) {
	const h = createHash("sha256");
	h.update(model ?? "");
	h.update([...toolCallIds].sort().join(","));
	return h.digest("hex");
}

function rememberTurn(model, toolCallIds, items) {
	trimCache();
	turnCache.set(turnKey(model, toolCallIds), { items, expiresAt: Date.now() + CACHE_TTL_MS });
}

function recallTurn(model, toolCallIds) {
	const entry = turnCache.get(turnKey(model, toolCallIds));
	if (!entry || entry.expiresAt < Date.now()) return undefined;
	return entry.items;
}

function chatToolsToResponsesTools(tools) {
	if (!Array.isArray(tools)) return undefined;
	return tools.map((t) => ({
		type: "function",
		name: t.function.name,
		description: t.function.description,
		parameters: t.function.parameters,
		strict: false,
	}));
}

function chatToolChoiceToResponses(toolChoice) {
	if (toolChoice == null) return undefined;
	if (typeof toolChoice === "string") return toolChoice;
	if (toolChoice.type === "function") return { type: "function", name: toolChoice.function?.name };
	return undefined;
}

/** Translate a full Chat Completions messages array into a Responses API {instructions, input}, splicing in cached reasoning items for any tool-call turn we recognize. */
function buildResponsesInput(messages, model) {
	let instructions;
	const input = [];
	for (const m of messages) {
		if (m.role === "system" || m.role === "developer") {
			instructions = instructions ? `${instructions}\n\n${m.content}` : m.content;
			continue;
		}
		if (m.role === "user") {
			input.push({ role: "user", content: typeof m.content === "string" ? m.content : JSON.stringify(m.content) });
			continue;
		}
		if (m.role === "assistant") {
			if (Array.isArray(m.tool_calls) && m.tool_calls.length > 0) {
				const toolCallIds = m.tool_calls.map((t) => t.id);
				const cachedItems = recallTurn(model, toolCallIds);
				if (cachedItems) {
					input.push(...cachedItems.filter((item) => item.type === "reasoning" || item.type === "function_call"));
				} else {
					// Cache miss (process restart, TTL, or a turn we never produced) —
					// best-effort reconstruction without reasoning items. Same exposure
					// as the original bug for this one leg, but doesn't hard-fail.
					for (const tc of m.tool_calls) {
						input.push({ type: "function_call", call_id: tc.id, name: tc.function.name, arguments: tc.function.arguments });
					}
				}
				if (m.content) input.push({ role: "assistant", content: m.content });
			} else if (m.content) {
				input.push({ role: "assistant", content: m.content });
			}
			continue;
		}
		if (m.role === "tool") {
			input.push({ type: "function_call_output", call_id: m.tool_call_id, output: typeof m.content === "string" ? m.content : JSON.stringify(m.content) });
			continue;
		}
	}
	return { instructions, input };
}

function responsesOutputToChatMessage(output) {
	let content = "";
	const toolCalls = [];
	for (const item of output ?? []) {
		if (item.type === "message" && item.role === "assistant") {
			for (const part of item.content ?? []) {
				if (part.type === "output_text" && typeof part.text === "string") content += part.text;
			}
		} else if (item.type === "function_call") {
			toolCalls.push({ id: item.call_id, type: "function", function: { name: item.name, arguments: item.arguments } });
		}
	}
	const message = { role: "assistant", content: content || null };
	if (toolCalls.length > 0) message.tool_calls = toolCalls;
	return message;
}

function toChatCompletionResponse(responsesBody, requestedModel) {
	const message = responsesOutputToChatMessage(responsesBody.output);
	const finishReason = message.tool_calls ? "tool_calls" : responsesBody.status === "incomplete" ? "length" : "stop";
	const usage = responsesBody.usage ?? {};
	return {
		id: responsesBody.id ?? `chatcmpl_${randomUUID()}`,
		object: "chat.completion",
		created: responsesBody.created_at ?? Math.floor(Date.now() / 1000),
		model: responsesBody.model ?? requestedModel,
		choices: [{ index: 0, message, finish_reason: finishReason }],
		usage: {
			prompt_tokens: usage.input_tokens ?? 0,
			completion_tokens: usage.output_tokens ?? 0,
			total_tokens: usage.total_tokens ?? 0,
		},
	};
}

function reasoningEffortFromBody(body) {
	const effort = body.reasoning_effort;
	if (!effort || effort === "none" || effort === "off") return undefined;
	return { effort, summary: "auto" };
}

/**
 * Upstream requires stream:true unconditionally. With store:false, the final
 * response.completed snapshot's own `.output` field comes back empty — the
 * incremental response.output_item.done events are the actual source of
 * truth for output content, confirmed by direct testing (a real turn made 6
 * "read" function_calls visible only via output_item.done; the completed
 * snapshot's .output was []). So we accumulate output items ourselves from
 * output_item.done, and only use response.completed for top-level metadata
 * (status/usage/id/timestamps).
 */
async function collectCompletedResponseFromSse(body) {
	const reader = body.getReader();
	const decoder = new TextDecoder();
	let buffer = "";
	let completedMeta;
	let lastError;
	const itemsByIndex = new Map();
	while (true) {
		const { done, value } = await reader.read();
		if (done) break;
		buffer += decoder.decode(value, { stream: true });
		let idx;
		while ((idx = buffer.indexOf("\n\n")) !== -1) {
			const chunk = buffer.slice(0, idx);
			buffer = buffer.slice(idx + 2);
			for (const line of chunk.split("\n")) {
				if (!line.startsWith("data:")) continue;
				const payload = line.slice(5).trim();
				if (payload === "[DONE]" || payload === "") continue;
				let event;
				try {
					event = JSON.parse(payload);
				} catch {
					continue;
				}
				if (process.env.PROXY_VERBOSE === "1") console.log(`sse event: ${event.type}`);
				if (event.type === "response.output_item.done") {
					itemsByIndex.set(event.output_index, event.item);
				}
				if (event.type === "response.completed") completedMeta = event.response;
				if (event.type === "response.failed") lastError = event.response?.error ?? event;
				if (event.type === "error") lastError = event;
			}
		}
	}
	if (!completedMeta) {
		throw new Error(`upstream stream ended without response.completed${lastError ? `: ${JSON.stringify(lastError)}` : ""}`);
	}
	const output = [...itemsByIndex.entries()].sort((a, b) => a[0] - b[0]).map(([, item]) => item);
	return { ...completedMeta, output: output.length > 0 ? output : (completedMeta.output ?? []) };
}

async function callUpstreamResponses(client, body) {
	const resp = await client.request("/responses", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ ...body, stream: true }),
	});
	if (!resp.ok) {
		const text = await resp.text();
		const err = new Error(`upstream ${resp.status}: ${text.slice(0, 500)}`);
		err.status = resp.status;
		err.body = text;
		throw err;
	}
	return collectCompletedResponseFromSse(resp.body);
}

async function handleChatCompletions(body, client) {
	const model = body.model ?? "gpt-5.4";
	const tools = chatToolsToResponsesTools(body.tools);
	const toolChoice = chatToolChoiceToResponses(body.tool_choice);
	const reasoning = reasoningEffortFromBody(body);

	const { instructions, input } = buildResponsesInput(body.messages, model);
	const upstreamBody = {
		model,
		instructions,
		input,
		store: false,
		...(reasoning ? { include: ["reasoning.encrypted_content"] } : {}),
		...(tools ? { tools } : {}),
		...(toolChoice ? { tool_choice: toolChoice } : {}),
		...(reasoning ? { reasoning } : {}),
		...(typeof body.parallel_tool_calls === "boolean" ? { parallel_tool_calls: body.parallel_tool_calls } : {}),
	};

	const responsesResult = await callUpstreamResponses(client, upstreamBody);
	const chatResponse = toChatCompletionResponse(responsesResult, model);

	const toolCalls = chatResponse.choices[0].message.tool_calls;
	if (toolCalls && toolCalls.length > 0) {
		rememberTurn(model, toolCalls.map((t) => t.id), responsesResult.output ?? []);
	}

	if (process.env.PROXY_VERBOSE === "1") {
		const outputTypes = (responsesResult.output ?? []).map((o) => o.type);
		console.log(
			JSON.stringify({
				dbg: "turn",
				inputMessageCount: body.messages.length,
				inputItemCount: input.length,
				outputTypes,
				status: responsesResult.status,
				incompleteReason: responsesResult.incomplete_details,
				contentLen: (chatResponse.choices[0].message.content ?? "").length,
				toolCallCount: toolCalls?.length ?? 0,
				usage: responsesResult.usage,
			}),
		);
	}

	return chatResponse;
}

/** Bifrost/openclaw request stream:true on the Chat Completions side even though we
 * collect the full upstream response before replying. Emit it as a single-chunk SSE
 * stream so streaming clients get a valid response shape instead of an error. */
function writeChatCompletionChunkStream(res, chatResponse) {
	res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-cache", Connection: "keep-alive" });
	const base = { id: chatResponse.id, object: "chat.completion.chunk", created: chatResponse.created, model: chatResponse.model };
	const message = chatResponse.choices[0].message;
	res.write(`data: ${JSON.stringify({ ...base, choices: [{ index: 0, delta: { role: "assistant" }, finish_reason: null }] })}\n\n`);
	const delta = {};
	if (message.content) delta.content = message.content;
	if (message.tool_calls) delta.tool_calls = message.tool_calls.map((tc, i) => ({ index: i, id: tc.id, type: tc.type, function: tc.function }));
	if (Object.keys(delta).length > 0) {
		res.write(`data: ${JSON.stringify({ ...base, choices: [{ index: 0, delta, finish_reason: null }] })}\n\n`);
	}
	res.write(`data: ${JSON.stringify({ ...base, choices: [{ index: 0, delta: {}, finish_reason: chatResponse.choices[0].finish_reason }], usage: chatResponse.usage })}\n\n`);
	res.write("data: [DONE]\n\n");
	res.end();
}

async function readJsonBody(req) {
	const chunks = [];
	for await (const chunk of req) chunks.push(chunk);
	return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

async function main() {
	const client = createCodexOAuthClient({ authFilePath: OAUTH_FILE, responsesState: false });

	const server = createServer(async (req, res) => {
		try {
			if (req.method === "POST" && req.url === "/v1/chat/completions") {
				const body = await readJsonBody(req);
				const result = await handleChatCompletions(body, client);
				if (body.stream === true) {
					writeChatCompletionChunkStream(res, result);
				} else {
					res.writeHead(200, { "Content-Type": "application/json" });
					res.end(JSON.stringify(result));
				}
				return;
			}
			if (req.method === "GET" && req.url?.startsWith("/v1/models")) {
				const configuredModels = process.env.MODELS ? process.env.MODELS.split(",").map((m) => m.trim()) : undefined;
				const models = await resolveOpenAIOAuthModels(client, configuredModels, {
					codexVersion: process.env.CODEX_VERSION,
				});
				res.writeHead(200, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ object: "list", data: models.map((id) => ({ id, object: "model", created: 0, owned_by: "codex-oauth" })) }));
				return;
			}
			if (req.method === "GET" && req.url === "/healthz") {
				res.writeHead(200, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ ok: true, cacheSize: turnCache.size }));
				return;
			}
			res.writeHead(404, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: { message: "Route not found.", type: "not_found_error" } }));
		} catch (error) {
			console.error("request failed:", error);
			res.writeHead(error.status && error.status >= 400 && error.status < 600 ? error.status : 500, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: { message: error instanceof Error ? error.message : "Unexpected server error.", type: "server_error" } }));
		}
	});

	server.listen(PORT, HOST, () => {
		console.log(`continuation-aware codex proxy listening at http://${HOST}:${PORT}`);
	});
}

main().catch((error) => {
	console.error(error);
	process.exit(1);
});
