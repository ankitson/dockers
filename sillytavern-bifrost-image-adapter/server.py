#!/usr/bin/env python3
"""A1111-compatible image API shim for SillyTavern and local image backends."""

from __future__ import annotations

import base64
import json
import os
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any


HOST = os.environ.get("HOST", "0.0.0.0")
PORT = int(os.environ.get("PORT", "7860"))
BIFROST_BASE_URL = os.environ.get("BIFROST_BASE_URL", "").rstrip("/")
IMAGE_BACKEND = os.environ.get("IMAGE_BACKEND", "bifrost").strip().lower()
DEFAULT_MODEL = os.environ.get("IMAGE_MODEL", "").strip()
MODEL_LIST = [item.strip() for item in os.environ.get("IMAGE_MODELS", DEFAULT_MODEL).split(",") if item.strip()]
API_KEY = os.environ.get("BIFROST_API_KEY", "bifrost-local")
OPENROUTER_API_KEY = os.environ.get("OPENROUTER_API_KEY", "")
OPENROUTER_HTTP_REFERER = os.environ.get("OPENROUTER_HTTP_REFERER", "").strip()
QUALITY = os.environ.get("IMAGE_QUALITY", "").strip()
LOG_PROMPTS = os.environ.get("IMAGE_LOG_PROMPTS", "").strip().lower() in {"1", "true", "yes", "on"}
COMFYUI_BASE_URL = os.environ.get("COMFYUI_BASE_URL", "").rstrip("/")
COMFYUI_STEPS = int(os.environ.get("COMFYUI_STEPS", "6"))
COMFYUI_CFG = float(os.environ.get("COMFYUI_CFG", "2.0"))
COMFYUI_SAMPLER = os.environ.get("COMFYUI_SAMPLER", "euler")
COMFYUI_SCHEDULER = os.environ.get("COMFYUI_SCHEDULER", "simple")
COMFYUI_DENOISE = float(os.environ.get("COMFYUI_DENOISE", "1.0"))
COMFYUI_TIMEOUT = int(os.environ.get("COMFYUI_TIMEOUT", "300"))

STATE_LOCK = threading.Lock()
CURRENT_OPTIONS: dict[str, Any] = {"sd_model_checkpoint": DEFAULT_MODEL} if DEFAULT_MODEL else {}


def closest_openai_size(width: int, height: int) -> str:
    if width > height:
        return "1536x1024"
    if height > width:
        return "1024x1536"
    return "1024x1024"


def json_bytes(data: Any) -> bytes:
    return json.dumps(data, separators=(",", ":")).encode("utf-8")


def data_url_to_base64(value: str) -> str:
    if value.startswith("data:") and ";base64," in value:
        return value.split(";base64,", 1)[1]
    with urllib.request.urlopen(value, timeout=120) as response:
        return base64.b64encode(response.read()).decode("ascii")


def read_json_url(url: str, timeout: int = 30) -> Any:
    with urllib.request.urlopen(url, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def get_options() -> dict[str, Any]:
    with STATE_LOCK:
        return dict(CURRENT_OPTIONS)


def set_options(options: dict[str, Any]) -> None:
    if not isinstance(options, dict):
        return
    with STATE_LOCK:
        CURRENT_OPTIONS.update(options)


def comfy_object_info() -> dict[str, Any]:
    if not COMFYUI_BASE_URL:
        return {}
    try:
        data = read_json_url(f"{COMFYUI_BASE_URL}/object_info", timeout=10)
        if isinstance(data, dict):
            return data
    except Exception as exc:
        print(f"ComfyUI object_info failed: {exc}", file=sys.stderr, flush=True)
    return {}


def comfy_choices(node: str, field: str, fallback: list[str]) -> list[str]:
    info = comfy_object_info()
    try:
        choices = info[node]["input"]["required"][field][0]
    except Exception:
        return fallback
    return choices if isinstance(choices, list) and choices else fallback


def normalize_dimension(value: int) -> int:
    return max(64, int(round(value / 8)) * 8)


def generate_image(payload: dict[str, Any]) -> str:
    prompt = str(payload.get("prompt") or "").strip()
    if not prompt:
        raise ValueError("prompt is required")

    override_settings = payload.get("override_settings")
    if not isinstance(override_settings, dict):
        override_settings = {}
    model = str(
        override_settings.get("sd_model_checkpoint")
        or payload.get("model")
        or get_options().get("sd_model_checkpoint")
        or DEFAULT_MODEL
        or ""
    ).strip()
    if not model and IMAGE_BACKEND == "comfyui":
        choices = comfy_choices("CheckpointLoaderSimple", "ckpt_name", MODEL_LIST)
        model = choices[0] if choices else ""
    if not model:
        raise RuntimeError("IMAGE_MODEL or an equivalent request model is required")
    width = int(payload.get("width") or 1024)
    height = int(payload.get("height") or 1024)

    if LOG_PROMPTS:
        print(
            json.dumps(
                {
                    "event": "image_prompt",
                    "backend": IMAGE_BACKEND,
                    "model": model,
                    "width": width,
                    "height": height,
                    "prompt": prompt,
                    "negative_prompt": payload.get("negative_prompt") or "",
                },
                ensure_ascii=False,
            ),
            flush=True,
        )

    if IMAGE_BACKEND == "openrouter":
        return generate_openrouter_image(prompt, model, width, height)
    if IMAGE_BACKEND == "comfyui":
        return generate_comfyui_image(
            prompt=prompt,
            negative_prompt=str(payload.get("negative_prompt") or ""),
            model=model,
            width=width,
            height=height,
            payload=payload,
        )

    if not BIFROST_BASE_URL:
        raise RuntimeError("BIFROST_BASE_URL is required for the Bifrost backend")

    body: dict[str, Any] = {
        "model": model,
        "prompt": prompt,
        "size": closest_openai_size(width, height),
        "n": 1,
    }
    if QUALITY:
        body["quality"] = QUALITY

    request = urllib.request.Request(
        f"{BIFROST_BASE_URL}/images/generations",
        data=json_bytes(body),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {API_KEY}",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            data = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Bifrost image generation failed: HTTP {exc.code}: {detail}") from exc

    first = (data.get("data") or [{}])[0]
    if first.get("b64_json"):
        return first["b64_json"]
    if first.get("url"):
        return data_url_to_base64(first["url"])

    raise RuntimeError(f"Bifrost returned no image data: {json.dumps(data)[:1000]}")


def closest_aspect_ratio(width: int, height: int) -> str:
    ratio = width / max(height, 1)
    candidates = {
        "1:1": 1.0,
        "4:3": 4 / 3,
        "3:4": 3 / 4,
        "16:9": 16 / 9,
        "9:16": 9 / 16,
    }
    return min(candidates, key=lambda item: abs(candidates[item] - ratio))


def generate_openrouter_image(prompt: str, model: str, width: int, height: int) -> str:
    if not OPENROUTER_API_KEY:
        raise RuntimeError("OPENROUTER_API_KEY is required for IMAGE_BACKEND=openrouter")

    body = {
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "modalities": ["image"],
        "image_config": {"aspect_ratio": closest_aspect_ratio(width, height)},
    }
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {OPENROUTER_API_KEY}",
        "X-Title": "sillytavern-bifrost-image-adapter",
    }
    if OPENROUTER_HTTP_REFERER:
        headers["HTTP-Referer"] = OPENROUTER_HTTP_REFERER

    request = urllib.request.Request(
        "https://openrouter.ai/api/v1/chat/completions",
        data=json_bytes(body),
        headers=headers,
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            data = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"OpenRouter image generation failed: HTTP {exc.code}: {detail}") from exc

    image_url = data.get("choices", [{}])[0].get("message", {}).get("images", [{}])[0].get("image_url", {}).get("url")
    if not image_url:
        raise RuntimeError(f"OpenRouter returned no image data: {json.dumps(data)[:1000]}")

    return data_url_to_base64(image_url)


def generate_comfyui_image(
    prompt: str,
    negative_prompt: str,
    model: str,
    width: int,
    height: int,
    payload: dict[str, Any],
) -> str:
    if not COMFYUI_BASE_URL:
        raise RuntimeError("COMFYUI_BASE_URL is required for IMAGE_BACKEND=comfyui")
    width = normalize_dimension(width)
    height = normalize_dimension(height)
    seed = int(payload.get("seed") if payload.get("seed") not in {None, -1, "-1"} else time.time_ns() % (2**63))
    steps = int(payload.get("steps") or COMFYUI_STEPS)
    cfg = float(payload.get("cfg_scale") or COMFYUI_CFG)
    sampler = str(payload.get("sampler_name") or COMFYUI_SAMPLER)
    scheduler = str(payload.get("scheduler") or COMFYUI_SCHEDULER)

    workflow = {
        "4": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": model}},
        "5": {"class_type": "EmptyLatentImage", "inputs": {"width": width, "height": height, "batch_size": 1}},
        "6": {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": ["4", 1]}},
        "7": {"class_type": "CLIPTextEncode", "inputs": {"text": negative_prompt, "clip": ["4", 1]}},
        "3": {
            "class_type": "KSampler",
            "inputs": {
                "seed": seed,
                "steps": steps,
                "cfg": cfg,
                "sampler_name": sampler,
                "scheduler": scheduler,
                "denoise": COMFYUI_DENOISE,
                "model": ["4", 0],
                "positive": ["6", 0],
                "negative": ["7", 0],
                "latent_image": ["5", 0],
            },
        },
        "8": {"class_type": "VAEDecode", "inputs": {"samples": ["3", 0], "vae": ["4", 2]}},
        "9": {"class_type": "SaveImage", "inputs": {"filename_prefix": "sillytavern_comfyui", "images": ["8", 0]}},
    }

    request = urllib.request.Request(
        f"{COMFYUI_BASE_URL}/prompt",
        data=json_bytes({"prompt": workflow, "client_id": str(uuid.uuid4())}),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            queued = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"ComfyUI prompt failed: HTTP {exc.code}: {detail}") from exc

    if queued.get("node_errors"):
        raise RuntimeError(f"ComfyUI workflow errors: {json.dumps(queued['node_errors'])[:1000]}")
    prompt_id = queued.get("prompt_id")
    if not prompt_id:
        raise RuntimeError(f"ComfyUI returned no prompt_id: {json.dumps(queued)[:1000]}")

    deadline = time.monotonic() + COMFYUI_TIMEOUT
    history: dict[str, Any] | None = None
    while time.monotonic() < deadline:
        data = read_json_url(f"{COMFYUI_BASE_URL}/history/{urllib.parse.quote(prompt_id)}", timeout=30)
        if isinstance(data, dict) and prompt_id in data:
            history = data[prompt_id]
            break
        time.sleep(1)
    if history is None:
        raise RuntimeError(f"ComfyUI timed out waiting for prompt {prompt_id}")

    status = history.get("status", {})
    if status.get("status_str") != "success" or not status.get("completed"):
        raise RuntimeError(f"ComfyUI generation did not complete: {json.dumps(status)[:1000]}")

    images: list[dict[str, Any]] = []
    for output in history.get("outputs", {}).values():
        if isinstance(output, dict):
            images.extend(output.get("images") or [])
    if not images:
        raise RuntimeError(f"ComfyUI returned no images for prompt {prompt_id}")

    first = images[0]
    query = urllib.parse.urlencode(
        {
            "filename": first.get("filename", ""),
            "subfolder": first.get("subfolder", ""),
            "type": first.get("type", "output"),
        }
    )
    with urllib.request.urlopen(f"{COMFYUI_BASE_URL}/view?{query}", timeout=60) as response:
        return base64.b64encode(response.read()).decode("ascii")


def model_entries() -> list[dict[str, Any]]:
    models = MODEL_LIST
    if IMAGE_BACKEND == "comfyui":
        models = comfy_choices("CheckpointLoaderSimple", "ckpt_name", MODEL_LIST)
    return [
        {
            "title": model,
            "model_name": model,
            "filename": model,
            "hash": None,
            "sha256": None,
            "config": None,
        }
        for model in models
    ]


def sampler_entries() -> list[dict[str, str]]:
    if IMAGE_BACKEND == "comfyui":
        return [{"name": item} for item in comfy_choices("KSampler", "sampler_name", [COMFYUI_SAMPLER])]
    return [{"name": "Bifrost"}]


def scheduler_entries() -> list[dict[str, str]]:
    if IMAGE_BACKEND == "comfyui":
        return [{"name": item} for item in comfy_choices("KSampler", "scheduler", [COMFYUI_SCHEDULER])]
    return [{"name": "normal"}]


class Handler(BaseHTTPRequestHandler):
    server_version = "sillytavern-bifrost-image-adapter/1.0"

    def log_message(self, fmt: str, *args: Any) -> None:
        print(f"{self.address_string()} - {fmt % args}", file=sys.stderr, flush=True)

    def read_json(self) -> dict[str, Any]:
        length = int(self.headers.get("Content-Length") or "0")
        if length <= 0:
            return {}
        raw = self.rfile.read(length)
        return json.loads(raw.decode("utf-8"))

    def send_json(self, data: Any, status: int = 200) -> None:
        body = json_bytes(data)
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:
        if self.path == "/healthz":
            return self.send_json({"ok": True})
        if self.path == "/sdapi/v1/options":
            return self.send_json(get_options())
        if self.path == "/sdapi/v1/sd-models":
            return self.send_json(model_entries())
        if self.path == "/sdapi/v1/samplers":
            return self.send_json(sampler_entries())
        if self.path == "/sdapi/v1/schedulers":
            return self.send_json(scheduler_entries())
        if self.path == "/sdapi/v1/upscalers":
            return self.send_json([{"name": "None"}])
        if self.path == "/sdapi/v1/sd-vae":
            return self.send_json([{"model_name": "Automatic", "filename": "Automatic"}])
        if self.path == "/sdapi/v1/sd-modules":
            return self.send_json([])
        if self.path == "/sdapi/v1/latent-upscale-modes":
            return self.send_json([{"name": "Latent"}])
        if self.path == "/sdapi/v1/progress":
            return self.send_json({"progress": 0.0, "eta_relative": 0.0, "state": {"job_count": 0}})
        return self.send_json({"error": "not found"}, 404)

    def do_POST(self) -> None:
        try:
            if self.path == "/sdapi/v1/options":
                payload = self.read_json()
                set_options(payload)
                if "sd_model_checkpoint" in payload:
                    print(f"model switched to {payload['sd_model_checkpoint']}", flush=True)
                return self.send_json({"ok": True})
            if self.path == "/sdapi/v1/interrupt":
                return self.send_json({"ok": True})
            if self.path == "/sdapi/v1/txt2img":
                payload = self.read_json()
                image = generate_image(payload)
                return self.send_json({"images": [image], "parameters": payload, "info": "{}"})
            return self.send_json({"error": "not found"}, 404)
        except Exception as exc:
            print(f"request failed: {exc}", file=sys.stderr, flush=True)
            return self.send_json({"error": str(exc)}, 502)


def main() -> None:
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    print(
        f"listening on {HOST}:{PORT}; backend={IMAGE_BACKEND}; bifrost={BIFROST_BASE_URL}; "
        f"comfyui={COMFYUI_BASE_URL}; model={DEFAULT_MODEL}",
        flush=True,
    )
    server.serve_forever()


if __name__ == "__main__":
    main()
