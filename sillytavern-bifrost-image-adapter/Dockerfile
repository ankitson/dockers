FROM python:3.12-alpine

WORKDIR /app
COPY server.py /app/server.py

ENV HOST=0.0.0.0
ENV PORT=7860
ENV IMAGE_BACKEND=bifrost

CMD ["python", "/app/server.py"]
