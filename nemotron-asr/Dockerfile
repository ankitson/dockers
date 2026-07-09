# Nemotron 3.5 ASR — parakeet-rs ONNX Runtime server
#
# Serves POST /v1/audio/transcriptions (OpenAI-compatible) using NVIDIA's
# Nemotron 3.5 ASR model (streaming, multilingual, 0.6B) via parakeet-rs
# and ONNX Runtime with CUDA execution provider.

FROM nvidia/cuda:12.6.3-runtime-ubuntu24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY nemotron-asr-server/target/release/nemotron-asr-server /app/server
COPY models/nemotron/ /models/nemotron/
COPY models/parakeet-ctc/ /models/parakeet-ctc/

ENV MODEL_DIRS=/models/nemotron,/models/parakeet-ctc

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=30s \
    CMD curl -f http://localhost:8000/health || exit 1

CMD ["/app/server"]
