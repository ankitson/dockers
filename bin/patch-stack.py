# /// script
# requires-python = ">=3.11"
# ///
"""Inspect local patch-stack metadata without touching containers."""

from __future__ import annotations

import argparse
import sys
import tomllib
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "patches.toml"


def load_manifest() -> dict[str, Any]:
    with MANIFEST.open("rb") as handle:
        return tomllib.load(handle)


def iter_patches(manifest: dict[str, Any]):
    for stack in manifest.get("stacks", []):
        for patch in stack.get("patches", []):
            yield stack, patch


def list_patches(manifest: dict[str, Any]) -> int:
    for stack in manifest.get("stacks", []):
        print(f"{stack['id']} ({stack.get('upstream_kind', '?')}:{stack.get('upstream', '?')})")
        for patch in stack.get("patches", []):
            path = patch.get("path", "<inline>")
            print(f"  - {patch['id']} [{patch.get('type', '?')}] {path}")
    return 0


def check_manifest(manifest: dict[str, Any]) -> int:
    errors: list[str] = []
    seen_ids: set[str] = set()

    for stack in manifest.get("stacks", []):
        stack_id = stack.get("id")
        if not stack_id:
            errors.append("stack missing id")
            continue
        owner_path = ROOT / stack.get("owner_path", "")
        if not owner_path.is_dir():
            errors.append(f"{stack_id}: owner_path does not exist: {owner_path.relative_to(ROOT)}")

        dockerfile = owner_path / "Dockerfile"
        if not dockerfile.is_file():
            errors.append(f"{stack_id}: missing Dockerfile at {dockerfile.relative_to(ROOT)}")

        for patch in stack.get("patches", []):
            patch_id = patch.get("id")
            if not patch_id:
                errors.append(f"{stack_id}: patch missing id")
                continue
            full_id = f"{stack_id}:{patch_id}"
            if full_id in seen_ids:
                errors.append(f"{full_id}: duplicate patch id")
            seen_ids.add(full_id)

            rel_path = patch.get("path")
            patch_type = patch.get("type", "")
            if rel_path:
                path = ROOT / rel_path
                if not path.is_file():
                    errors.append(f"{full_id}: missing patch file: {rel_path}")
                    continue
                text = path.read_text(encoding="utf-8", errors="replace")
                if patch_type == "git-apply" and "diff --git " not in text:
                    errors.append(f"{full_id}: git-apply patch does not contain a git diff header")
                if patch_type.endswith("script") and not text.startswith("#!"):
                    errors.append(f"{full_id}: patch script should start with a shebang")

            for apply_ref in patch.get("applied_by", []):
                apply_path = ROOT / apply_ref.split(":", 1)[0]
                if not apply_path.exists():
                    errors.append(f"{full_id}: applied_by target missing: {apply_ref}")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print(f"OK: {sum(1 for _ in iter_patches(manifest))} patches across {len(manifest.get('stacks', []))} stacks")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["list", "check"])
    args = parser.parse_args()

    manifest = load_manifest()
    if args.command == "list":
        return list_patches(manifest)
    if args.command == "check":
        return check_manifest(manifest)
    raise AssertionError(args.command)


if __name__ == "__main__":
    raise SystemExit(main())
