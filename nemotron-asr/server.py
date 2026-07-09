"""Minimal OpenAI-compatible /v1/audio/transcriptions server for NVIDIA Nemotron 3.5 ASR.

Loads a NeMo ASR model once, then transcribes uploaded audio via the OpenAI
multipart contract (field `file`, optional `model`). Returns `{"text": "..."}`.
"""
import io
import os
import tempfile
import wave

from fastapi import FastAPI, File, Form, UploadFile, HTTPException
from fastapi.responses import JSONResponse

app = FastAPI()

MODEL_ID = os.environ.get("NEMOTRON_MODEL", "nvidia/nemotron-3.5-asr-streaming-0.6b")
_MODEL = None


def get_model():
    global _MODEL
    if _MODEL is None:
        from nemo.collections.asr.models import ASRModel

        _MODEL = ASRModel.from_pretrained(MODEL_ID, map_location="cuda")
    return _MODEL


@app.on_event("startup")
def _warm():
    try:
        get_model()
    except Exception as exc:  # don't crash the server if model pull fails at boot
        print(f"[nemotron-asr] model preload skipped: {exc}")


def _to_wav16k_mono(raw: bytes) -> str:
    """Use ffmpeg to normalize any input audio to 16 kHz mono WAV; return path."""
    import subprocess

    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as out:
        out_path = out.name
    with tempfile.NamedTemporaryFile(suffix=".bin", delete=False) as inp:
        inp.write(raw)
        in_path = inp.name
    try:
        subprocess.run(
            [
                "ffmpeg", "-y", "-i", in_path,
                "-ac", "1", "-ar", "16000", "-f", "wav", out_path,
            ],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    finally:
        os.unlink(in_path)
    return out_path


@app.get("/health")
def health():
    return {"status": "ok", "model": MODEL_ID}


@app.post("/v1/audio/transcriptions")
async def transcriptions(file: UploadFile = File(...), model: str = Form("")):
    data = await file.read()
    if not data:
        raise HTTPException(status_code=400, detail="empty audio file")
    wav_path = _to_wav16k_mono(data)
    try:
        model_obj = get_model()
        # NeMo transcribe returns a list of transcribed strings.
        texts = model_obj.transcribe([wav_path], batch_size=1)
        text = texts[0] if isinstance(texts, list) and texts else ""
    finally:
        os.unlink(wav_path)
    return JSONResponse({"text": text})


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
