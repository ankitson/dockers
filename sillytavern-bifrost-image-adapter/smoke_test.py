#!/usr/bin/env python3
"""Smoke-test the local A1111-compatible image adapter."""

from __future__ import annotations

import base64
import json
import os
import urllib.request
from pathlib import Path

BASE_URL = os.environ.get("ST_IMAGE_ADAPTER_URL", "http://127.0.0.1:7862").rstrip("/")
PREFERRED_MODEL = os.environ.get("ST_IMAGE_TEST_MODEL", "").strip()


def request_json(path: str, payload: dict | None = None) -> dict | list:
    url = f"{BASE_URL}{path}"
    if payload is None:
        with urllib.request.urlopen(url, timeout=30) as response:
            return json.loads(response.read().decode())
    request = urllib.request.Request(
        url,
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=360) as response:
        return json.loads(response.read().decode())


def select_model() -> str:
    models = request_json("/sdapi/v1/sd-models")
    names = [str(item.get("title") or item.get("model_name") or "") for item in models if isinstance(item, dict)]
    if PREFERRED_MODEL in names:
        return PREFERRED_MODEL
    if not names:
        raise RuntimeError("adapter returned no models")
    return names[0]


def main() -> None:
    model = select_model()
    request_json("/sdapi/v1/options", {"sd_model_checkpoint": model})
    options = request_json("/sdapi/v1/options")
    if not isinstance(options, dict) or options.get("sd_model_checkpoint") != model:
        raise RuntimeError(f"model switch did not stick: {options!r}")

    payload = {
        "prompt": "a simple red square centered on a white background, clean flat graphic",
        "negative_prompt": "blurry, text, watermark",
        "width": 512,
        "height": 512,
        "steps": 6,
        "cfg_scale": 2,
    }
    data = request_json("/sdapi/v1/txt2img", payload)
    image = base64.b64decode(data["images"][0])
    output = Path("logs/sillytavern-image-adapter-smoke.png")
    output.parent.mkdir(exist_ok=True)
    output.write_bytes(image)
    print(f"model {model}")
    print(f"wrote {output} {len(image)} bytes {image[:8]!r}")


if __name__ == "__main__":
    main()
