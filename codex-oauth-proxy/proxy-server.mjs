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
const LOG_LEVEL = (() => {
	const explicit = process.env.PROXY_LOG_LEVEL ?? (process.env.PROXY_VERBOSE === "1" ? "debug" : "info");
	const normalized = explicit.toLowerCase();
	return normalized;
})();
const LOG_LEVELS = { error: 0, warn: 1, info: 2, debug: 3 };

function shouldLog(level) {
	const threshold = LOG_LEVELS[LOG_LEVEL];
	const candidate = LOG_LEVELS[level];
	if (typeof threshold !== "number" || typeof candidate !== "number") return level === "error" || level === "warn" || level === "info";
	return candidate <= threshold;
}

function clamp(text, maxLen) {
	if (text == null) return text;
	const s = typeof text === "string" ? text : JSON.stringify(text);
	if (typeof s !== "string" || s.length <= maxLen) return s;
	return `${s.slice(0, maxLen)}…(+${s.length - maxLen})`;
}

function log(level, message, details = {}) {
	if (!shouldLog(level)) return;
	try {
		console.log(
			JSON.stringify({
				ts: new Date().toISOString(),
				level,
				message,
				...details,
			}),
		);
	} catch {
		console.log(`${new Date().toISOString()} ${level} ${message}`, details);
	}
}

function summarizeMessages(messages = []) {
	const byRole = {};
	let toolTurnCount = 0;
	let toolCalls = 0;
	for (const message of messages) {
		if (!message || typeof message !== "object") continue;
		byRole[message.role] = (byRole[message.role] || 0) + 1;
		if (message.role === "assistant" && Array.isArray(message.tool_calls) && message.tool_calls.length > 0) {
			toolTurnCount += 1;
			toolCalls += message.tool_calls.length;
		}
	}
	return { total: messages.length, byRole, toolTurnCount, toolCalls };
}

function summarizeInputItems(items = [], limit = 12) {
	return items.slice(0, limit).map((item) => ({
		type: item?.type,
		role: item?.role,
		callId: item?.call_id,
		name: item?.name,
	}));
}

function describeSseBody(body = {}) {
	return {
		stream: body.stream,
		model: body.model,
		reasoningEffort: body.reasoning_effort,
		toolCount: Array.isArray(body.tools) ? body.tools.length : 0,
		parallelToolCalls: body.parallel_tool_calls,
		messageSummary: summarizeMessages(body.messages),
	};
}

function extractRequestId(req) {
	for (const key of ["x-request-id", "x-bf-request-id", "x-request-id-cf"]) {
		const raw = req.headers[key];
		if (!raw) continue;
		if (Array.isArray(raw)) return raw[0];
		if (typeof raw === "string") return raw;
	}
	return `req-${randomUUID()}`;
}

function normalizeUpstreamError(errorLike, messageFallback) {
	const raw = typeof errorLike === "string" ? { message: errorLike } : errorLike;
	if (!raw || typeof raw !== "object") {
		return { status: 500, message: messageFallback || "Upstream request failed.", type: "api_error", code: "upstream_error" };
	}
	const envelope = raw.error && typeof raw.error === "object" ? raw.error : raw;
	const code = envelope.code;
	const type = envelope.type;
	const message = envelope.message || messageFallback || "Upstream request failed.";
	let status = 500;
	let openAIType = type || "api_error";
	if (code === "context_length_exceeded") {
		status = 400;
		openAIType = "invalid_request_error";
	} else if (code === "rate_limit_exceeded") {
		status = 429;
		openAIType = "rate_limit_error";
	} else if (code === "insufficient_quota") {
		status = 403;
		openAIType = "insufficient_quota";
	}
	return {
		status,
		error: {
			message,
			type: openAIType,
			param: envelope.param ?? null,
			code,
		},
	};
}

function buildUpstreamErrorResponse(errorLike, messageFallback) {
	const normalized = normalizeUpstreamError(errorLike, messageFallback);
	return {
		status: normalized.status,
		error: normalized.error,
	};
}

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
async function collectCompletedResponseFromSse(body, requestContext = {}) {
	const requestId = requestContext.requestId || `req-${randomUUID()}`;
	const reader = body.getReader();
	const decoder = new TextDecoder();
	let buffer = "";
	let completedMeta;
	let lastError;
	let totalBytes = 0;
	let eventCount = 0;
	let parseErrors = 0;
	const eventCounts = {};
	const itemsByIndex = new Map();
	let lastEventType;
	let lastOutputIndex;
	log("debug", "upstream_sse_started", { requestId, model: requestContext.model });
	while (true) {
		const { done, value } = await reader.read();
		if (done) break;
		totalBytes += value?.byteLength || 0;
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
					parseErrors += 1;
				if (parseErrors <= 3) log("warn", "upstream_sse_parse_error", { requestId, payload: clamp(payload, 240), parseErrors });
					continue;
				}
				eventCount += 1;
				eventCounts[event.type] = (eventCounts[event.type] || 0) + 1;
				lastEventType = event.type;
				if (event.output_index !== undefined) lastOutputIndex = event.output_index;
				log("debug", "upstream_sse_event", { requestId, type: event.type, outputIndex: event.output_index, responseId: event.response?.id });
				if (event.type === "response.output_item.done") {
					itemsByIndex.set(event.output_index, event.item);
				}
				if (event.type === "response.completed") completedMeta = event.response;
				if (event.type === "response.failed") lastError = event.response?.error ?? event;
				if (event.type === "error") lastError = event;
			}
		}
	}
	log("info", "upstream_sse_finished", {
		requestId,
		totalBytes,
		eventCount,
		parseErrors,
		eventCounts,
		outputItemCount: itemsByIndex.size,
		lastEventType,
		lastOutputIndex,
		completed: !!completedMeta,
	});
	if (!completedMeta) {
		log("warn", "upstream_stream_ended_without_completed", {
			requestId,
			lastError: lastError ? clamp(lastError, 1200) : null,
			lastEventType,
			lastOutputIndex,
			eventCounts,
		});
		const upstreamError = buildUpstreamErrorResponse(lastError, "upstream stream ended without response.completed");
		const err = new Error(upstreamError.error.message);
		err.status = upstreamError.status;
		err.openaiError = upstreamError.error;
		throw err;
	}
	const output = [...itemsByIndex.entries()].sort((a, b) => a[0] - b[0]).map(([, item]) => item);
	log("debug", "upstream_items_assembled", { requestId, outputCount: output.length, sampleOutput: summarizeInputItems(output) });
	return { ...completedMeta, output: output.length > 0 ? output : (completedMeta.output ?? []) };
}

async function callUpstreamResponses(client, body, requestContext = {}) {
	const requestId = requestContext.requestId || `req-${randomUUID()}`;
	const startedAt = Date.now();
	log("info", "upstream_request_start", { requestId, model: body.model, inputCount: Array.isArray(body.input) ? body.input.length : 0, toolCount: Array.isArray(body.tools) ? body.tools.length : 0 });
	const resp = await client.request("/responses", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ ...body, stream: true }),
	});
	log("debug", "upstream_response_status", { requestId, status: resp.status, ok: resp.ok, contentType: resp.headers?.get?.("content-type"), latencyMs: Date.now() - startedAt });
	if (!resp.ok) {
		const text = await resp.text();
		log("error", "upstream_non_ok", { requestId, status: resp.status, body: clamp(text, 500) });
		let upstreamBody;
		try {
			upstreamBody = JSON.parse(text);
		} catch {
			// non-JSON bodies are still forwarded as a generic upstream failure
		}
		const upstreamError = buildUpstreamErrorResponse(
			upstreamBody ?? { code: resp.status >= 500 ? "api_error" : "invalid_request_error", message: text || `upstream ${resp.status}` },
			`upstream ${resp.status}: ${text.slice(0, 500)}`,
		);
		const err = new Error(upstreamError.error.message);
		err.status = upstreamError.status;
		err.openaiError = upstreamError.error;
		err.body = text;
		throw err;
	}
	const responsesResult = await collectCompletedResponseFromSse(resp.body, requestContext);
	log("debug", "upstream_response_complete", {
		requestId,
		responseId: responsesResult.id,
		status: responsesResult.status,
		model: responsesResult.model,
		usage: responsesResult.usage,
		latencyMs: Date.now() - startedAt,
	});
	return responsesResult;
}

async function handleChatCompletions(body, client, requestContext = {}) {
	const model = requestContext.model || body.model || "gpt-5.4";
	log("info", "chat_completion_start", {
		requestId: requestContext.requestId,
		model,
		stream: body.stream,
		reasoningEffort: body.reasoning_effort,
		messageSummary: summarizeMessages(body.messages),
	});
	const tools = chatToolsToResponsesTools(body.tools);
	const toolChoice = chatToolChoiceToResponses(body.tool_choice);
	const reasoning = reasoningEffortFromBody(body);

	const { instructions, input } = buildResponsesInput(body.messages, model);
	log("debug", "chat_completion_input", { requestId: requestContext.requestId, model, instructionLength: instructions?.length ?? 0, inputCount: input.length, toolCount: Array.isArray(tools) ? tools.length : 0, sampleInput: summarizeInputItems(input) });
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

	const requestContextWithModel = { ...requestContext, model };
	const responsesResult = await callUpstreamResponses(client, upstreamBody, requestContextWithModel);
	const chatResponse = toChatCompletionResponse(responsesResult, model);

	const toolCalls = chatResponse.choices[0].message.tool_calls;
	if (toolCalls && toolCalls.length > 0) {
		rememberTurn(model, toolCalls.map((t) => t.id), responsesResult.output ?? []);
	}

	const outputTypes = (responsesResult.output ?? []).map((o) => o.type);
	log("info", "chat_completion_end", {
		requestId: requestContext.requestId,
		responseId: responsesResult.id,
		model,
		status: responsesResult.status,
		incompleteReason: responsesResult.incomplete_details,
		finishReason: chatResponse.choices[0].finish_reason,
		contentLen: (chatResponse.choices[0].message.content ?? "").length,
		toolCallCount: toolCalls?.length ?? 0,
		usage: responsesResult.usage,
		outputTypes,
	});

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
	const requestId = req.__requestId || `req-${randomUUID()}`;
	const chunks = [];
	let totalBytes = 0;
	for await (const chunk of req) {
		totalBytes += chunk?.byteLength || 0;
		chunks.push(chunk);
	}
	const text = Buffer.concat(chunks).toString("utf8");
	log("debug", "request_body_read", { requestId, totalBytes, lineCount: text.split("\n").length });
	try {
		const body = JSON.parse(text);
		log("debug", "request_body_parsed", {
			requestId,
			descriptor: describeSseBody(body),
			truncatedModelText: body?.messages?.[0]?.content ? clamp(body.messages[0].content, 180) : null,
		});
		return body;
	} catch (error) {
		log("error", "request_body_parse_error", {
			requestId,
			raw: clamp(text, 300),
			error: error?.message,
		});
		throw error;
	}
}

async function main() {
	const client = createCodexOAuthClient({ authFilePath: OAUTH_FILE, responsesState: false });

	const server = createServer(async (req, res) => {
		try {
			const requestId = extractRequestId(req);
			req.__requestId = requestId;
			log("info", "request_started", {
				requestId,
				method: req.method,
				url: req.url,
				remoteAddress: req.socket.remoteAddress,
			});
			if (req.method === "POST" && req.url === "/v1/chat/completions") {
				const body = await readJsonBody(req);
				const result = await handleChatCompletions(body, client, {
					requestId,
					startedAt: Date.now(),
					remoteAddress: req.socket.remoteAddress,
					stream: body.stream,
					model: body.model,
				});
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
			const openaiError = error?.openaiError || buildUpstreamErrorResponse(error, "Unexpected server error.").error;
			const statusCode = error?.status && error.status >= 400 && error.status < 600 ? error.status : 500;
			log("error", "request_failed", { requestId: req.__requestId, error: error instanceof Error ? error.message : "Unexpected server error." });
			console.error("request failed:", error);
			res.writeHead(statusCode, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: openaiError }));
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
