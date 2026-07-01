# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""Realistic test fixtures for the codex-oauth reasoning-continuity bug.

Builds a large-scale system prompt + tool schema from a synthetic workspace
file (checked in under tests/data/) plus a synthesized ~50-tool schema sized
to match a real agent framework's tool list byte-for-byte (names/sizes
captured from a live systemPromptReport dump), so tests reproduce the same
failure conditions as the real agent without needing the full stack.
"""

from pathlib import Path

DATA_DIR = Path(__file__).parent / "data"

# name -> (schemaChars, propertiesCount) captured from a real
# systemPromptReport.tools.entries dump for a couple of agents.
REAL_TOOL_SIZES = {
	"read": (304, 3),
	"edit": (834, 2),
	"write": (225, 2),
	"apply_patch": (153, 1),
	"exec": (1283, 12),
	"process": (1074, 12),
	"nodes": (1782, 32),
	"cron": (8705, 15),
	"message": (6166, 106),
	"tts": (264, 3),
	"image_generate": (2264, 16),
	"music_generate": (1015, 10),
	"video_generate": (2042, 18),
	"gateway": (649, 14),
	"agents_list": (33, 0),
	"get_goal": (33, 0),
	"create_goal": (267, 2),
	"update_goal": (207, 2),
	"skill_workshop": (2273, 13),
	"sessions_list": (435, 9),
	"sessions_history": (162, 3),
	"sessions_send": (275, 5),
	"sessions_spawn": (1476, 17),
	"sessions_yield": (60, 1),
	"subagents": (122, 2),
	"session_status": (89, 2),
	"web_search": (991, 12),
	"web_fetch": (317, 3),
	"image": (350, 6),
	"pdf": (448, 7),
	"browser": (3570, 53),
	"canvas": (724, 18),
	"file_fetch": (609, 6),
	"dir_list": (724, 7),
	"dir_fetch": (767, 7),
	"file_write": (975, 7),
	"memory_search": (250, 4),
	"memory_get": (241, 4),
	"mcpproxy__call_tool_destructive": (1303, 5),
	"mcpproxy__call_tool_read": (1283, 5),
	"mcpproxy__call_tool_write": (1272, 5),
	"mcpproxy__code_execution": (1333, 4),
	"mcpproxy__list_registries": (47, 0),
	"mcpproxy__quarantine_security": (924, 3),
	"mcpproxy__read_cache": (359, 3),
	"mcpproxy__retrieve_tools": (1517, 9),
	"mcpproxy__search_servers": (546, 4),
	"mcpproxy__upstream_servers": (2843, 14),
}

# The two tools the health-watch instruction actually motivates the model to
# call — real shape (not padded), since relevance (not just bulk) is what
# triggers the model to attempt a tool call at all.
REAL_READ_TOOL = {
	"type": "function",
	"function": {
		"name": "read",
		"description": "Reads a file from the local filesystem. Returns cat -n formatted output with line numbers.",
		"parameters": {
			"type": "object",
			"properties": {
				"file_path": {"type": "string", "description": "Absolute path to the file"},
				"offset": {"type": "integer", "description": "Line to start reading from"},
				"limit": {"type": "integer", "description": "Number of lines to read"},
			},
			"required": ["file_path"],
		},
	},
}

REAL_EXEC_TOOL = {
	"type": "function",
	"function": {
		"name": "exec",
		"description": "Executes a shell command and returns its output. Use for running scripts, checking system state, or invoking CLI tools.",
		"parameters": {
			"type": "object",
			"properties": {
				"command": {"type": "string", "description": "The command to execute"},
				"description": {"type": "string", "description": "What this command does"},
				"timeout": {"type": "number", "description": "Timeout in milliseconds"},
				"cwd": {"type": "string"},
				"run_in_background": {"type": "boolean"},
				"env": {"type": "object"},
				"a": {"type": "string"},
				"b": {"type": "string"},
				"c": {"type": "string"},
				"d": {"type": "string"},
				"e": {"type": "string"},
				"f": {"type": "string"},
			},
			"required": ["command"],
		},
	},
}


def _padded_tool(name: str, schema_chars: int, prop_count: int) -> dict:
	"""Synthesize a dummy tool schema sized to match the real one's byte count."""
	base = {
		"type": "function",
		"function": {
			"name": name,
			"description": "",
			"parameters": {
				"type": "object",
				"properties": {f"arg{i}": {"type": "string"} for i in range(max(prop_count, 1))},
			},
		},
	}
	import json

	current = len(json.dumps(base))
	pad_needed = max(schema_chars - current, 0)
	base["function"]["description"] = "x" * pad_needed
	return base


def build_tool_schema() -> list[dict]:
	tools = [REAL_READ_TOOL, REAL_EXEC_TOOL]
	for name, (chars, props) in REAL_TOOL_SIZES.items():
		if name in ("read", "exec"):
			continue
		tools.append(_padded_tool(name, chars, props))
	return tools


def build_system_prompt() -> str:
	workspace = (DATA_DIR / "agent_workspace.txt").read_text()
	# ~28KB of skills-prompt-shaped filler, matching a real agent framework's
	# non-project-context scale (skill descriptions injected for every
	# registered skill regardless of relevance to this agent's task).
	skills = [f"skill-{i:02d}" for i in range(47)]
	skill_block = "\n\n".join(
		f"## Skill: {name}\nUse this skill when working with {name}-related tasks. "
		f"Triggers on mentions of {name}, its CLI, or its API. " * 3
		for name in skills
	)
	return f"{workspace}\n\n# Available Skills\n\n{skill_block}"


def build_health_watch_message() -> str:
	return (DATA_DIR / "health-watch.md").read_text()


def scale_test_first_message() -> str:
	return (
		"Run the health-watch loop: read /workspace/agents/watchdog/loops/health-watch.md "
		"and execute it. Observe only; never mutate without an approval reply. Stay quiet if "
		"nothing is new."
	)


if __name__ == "__main__":
	tools = build_tool_schema()
	prompt = build_system_prompt()
	import json

	print(f"system prompt chars: {len(prompt)}")
	print(f"tool schema chars: {len(json.dumps(tools))}")
	print(f"tool count: {len(tools)}")
