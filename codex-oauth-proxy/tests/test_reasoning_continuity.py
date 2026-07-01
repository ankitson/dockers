# /// script
# requires-python = ">=3.11"
# dependencies = ["httpx"]
# ///
"""Acceptance suite for the codex-oauth reasoning-continuity bug.

Targets a codex-oauth-compatible endpoint directly (bypassing the agent gateway)
so scenarios are fast and isolated. Each scenario runs N trials and reports a
pass/fail/empty breakdown. A "pass" means the model produced a real answer (or
a genuine tool call on a fresh turn) instead of neither.

Usage:
    uv run test_reasoning_continuity.py --base-url http://127.0.0.1:10531 --trials 20
    uv run test_reasoning_continuity.py --base-url http://127.0.0.1:9911 --trials 20  # new proxy
"""

import argparse
import json
import sys
from dataclasses import dataclass, field
from pathlib import Path

import httpx

sys.path.insert(0, str(Path(__file__).parent))
from fixtures import (
	build_system_prompt,
	build_tool_schema,
	scale_test_first_message,
)

SMALL_TOOLS = [
	{
		"type": "function",
		"function": {
			"name": "read_file",
			"description": "Read a file from disk",
			"parameters": {"type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"]},
		},
	},
	{
		"type": "function",
		"function": {
			"name": "run_command",
			"description": "Run a shell command",
			"parameters": {"type": "object", "properties": {"cmd": {"type": "string"}}, "required": ["cmd"]},
		},
	},
]


@dataclass
class TrialResult:
	ok: bool
	detail: str
	raw: dict = field(default_factory=dict)


@dataclass
class ScenarioReport:
	name: str
	trials: list[TrialResult]

	@property
	def n(self) -> int:
		return len(self.trials)

	@property
	def n_ok(self) -> int:
		return sum(1 for t in self.trials if t.ok)

	@property
	def rate(self) -> str:
		return f"{self.n_ok}/{self.n}"

	def print_report(self):
		status = "PASS" if self.n_ok == self.n else "FAIL"
		print(f"[{status}] {self.name}: {self.rate} passed")
		for i, t in enumerate(self.trials):
			if not t.ok:
				print(f"    trial {i}: {t.detail}")


def call_chat_completions(client: httpx.Client, base_url: str, messages: list, tools: list | None = None, reasoning_effort: str = "medium") -> dict:
	payload = {"model": "gpt-5.4", "messages": messages, "reasoning_effort": reasoning_effort, "stream": False}
	if tools:
		payload["tools"] = tools
	resp = client.post(f"{base_url}/v1/chat/completions", json=payload, timeout=60)
	resp.raise_for_status()
	return resp.json()


def scenario_fresh_no_tools(client: httpx.Client, base_url: str, trials: int) -> ScenarioReport:
	results = []
	for i in range(trials):
		r = call_chat_completions(
			client, base_url,
			[{"role": "user", "content": f"Tell me a random one-sentence fact. (trial {i})"}],
		)
		msg = r["choices"][0]["message"]
		content = msg.get("content") or ""
		results.append(TrialResult(ok=bool(content), detail=f"empty response: {json.dumps(msg)[:200]}" if not content else "", raw=r))
	return ScenarioReport("fresh single-turn, no tools", results)


def scenario_fresh_with_tools(client: httpx.Client, base_url: str, trials: int) -> ScenarioReport:
	results = []
	for i in range(trials):
		r = call_chat_completions(
			client, base_url,
			[
				{"role": "system", "content": "You are an ops agent. When asked to check something, use the read_file or run_command tool."},
				{"role": "user", "content": "Read the file /etc/hostname and tell me what is in it, then run the command uptime and report."},
			],
			tools=SMALL_TOOLS,
		)
		msg = r["choices"][0]["message"]
		ok = bool(msg.get("content")) or bool(msg.get("tool_calls"))
		results.append(TrialResult(ok=ok, detail="" if ok else f"neither content nor tool_calls: {json.dumps(msg)[:200]}", raw=r))
	return ScenarioReport("fresh single-turn, with tools", results)


def scenario_small_tool_roundtrip(client: httpx.Client, base_url: str, trials: int) -> ScenarioReport:
	results = []
	base_messages = [
		{"role": "system", "content": "You are an ops agent. When asked to check something, use the read_file or run_command tool."},
		{"role": "user", "content": "Read the file /etc/hostname and tell me what is in it, then run the command uptime and report."},
	]
	for i in range(trials):
		r1 = call_chat_completions(client, base_url, base_messages, tools=SMALL_TOOLS)
		msg1 = r1["choices"][0]["message"]
		tc = msg1.get("tool_calls")
		if not tc:
			results.append(TrialResult(ok=False, detail=f"step1 did not call a tool: {json.dumps(msg1)[:200]}"))
			continue
		tool_results = [
			{"role": "tool", "tool_call_id": t["id"], "content": "myhostname-01" if t["function"]["name"] == "read_file" else "10:32:01 up 3 days"}
			for t in tc
		]
		followup = base_messages + [msg1] + tool_results
		r2 = call_chat_completions(client, base_url, followup, tools=SMALL_TOOLS)
		msg2 = r2["choices"][0]["message"]
		ok = bool(msg2.get("content")) or bool(msg2.get("tool_calls"))
		results.append(TrialResult(ok=ok, detail="" if ok else f"step2 EMPTY: {json.dumps(msg2)[:300]} finish_reason={r2['choices'][0].get('finish_reason')}", raw=r2))
	return ScenarioReport("small-scale 2-step tool round trip", results)


def scenario_large_scale_roundtrip(client: httpx.Client, base_url: str, trials: int) -> ScenarioReport:
	results = []
	system_prompt = build_system_prompt()
	tools = build_tool_schema()
	first_message = scale_test_first_message()
	base_messages = [
		{"role": "system", "content": system_prompt},
		{"role": "user", "content": first_message},
	]
	for i in range(trials):
		r1 = call_chat_completions(client, base_url, base_messages, tools=tools)
		msg1 = r1["choices"][0]["message"]
		tc = msg1.get("tool_calls")
		if not tc:
			# The model answered directly without needing a tool — acceptable,
			# not the failure we're hunting, but note it wasn't a true round-trip test.
			ok = bool(msg1.get("content"))
			results.append(TrialResult(ok=ok, detail="" if ok else f"step1 EMPTY (no tool, no content): {json.dumps(msg1)[:300]}"))
			continue
		tool_results = []
		for t in tc:
			name = t["function"]["name"]
			if name == "read":
				content = "1  # Health-watch loop\n2  Check container status and logs.\n"
			elif name == "exec":
				content = "CONTAINER   STATUS\nagent-runtime    Up 3 days (healthy)\n"
			else:
				content = "ok"
			tool_results.append({"role": "tool", "tool_call_id": t["id"], "content": content})
		followup = base_messages + [msg1] + tool_results
		r2 = call_chat_completions(client, base_url, followup, tools=tools)
		msg2 = r2["choices"][0]["message"]
		ok = bool(msg2.get("content")) or bool(msg2.get("tool_calls"))
		results.append(TrialResult(ok=ok, detail="" if ok else f"step2 EMPTY: {json.dumps(msg2)[:300]} finish_reason={r2['choices'][0].get('finish_reason')}", raw=r2))
	return ScenarioReport("large-scale 2-step tool round trip", results)


ALL_SCENARIOS = {
	"fresh_no_tools": scenario_fresh_no_tools,
	"fresh_with_tools": scenario_fresh_with_tools,
	"small_roundtrip": scenario_small_tool_roundtrip,
	"large_scale_roundtrip": scenario_large_scale_roundtrip,
}


def main():
	parser = argparse.ArgumentParser()
	parser.add_argument("--base-url", default="http://127.0.0.1:10531")
	parser.add_argument("--trials", type=int, default=10)
	parser.add_argument("--scenario", choices=list(ALL_SCENARIOS.keys()), default=None, help="Run only this scenario")
	parser.add_argument("--large-scale-trials", type=int, default=None, help="Override trial count for the expensive large-scale scenario")
	args = parser.parse_args()

	reports = []
	with httpx.Client() as client:
		scenarios = [args.scenario] if args.scenario else list(ALL_SCENARIOS.keys())
		for name in scenarios:
			trials = args.trials
			if name == "large_scale_roundtrip" and args.large_scale_trials is not None:
				trials = args.large_scale_trials
			print(f"running {name} ({trials} trials)...", file=sys.stderr)
			report = ALL_SCENARIOS[name](client, args.base_url, trials)
			reports.append(report)
			report.print_report()

	print()
	total_ok = sum(r.n_ok for r in reports)
	total_n = sum(r.n for r in reports)
	all_pass = all(r.n_ok == r.n for r in reports)
	print(f"TOTAL: {total_ok}/{total_n} — {'ALL PASS' if all_pass else 'FAILURES PRESENT'}")
	sys.exit(0 if all_pass else 1)


if __name__ == "__main__":
	main()
