# syntax=docker/dockerfile:1
FROM python:3.12-slim
WORKDIR /app

# System dependencies for sentence-transformers / PyMuPDF
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir \
    --index-url https://download.pytorch.org/whl/cpu \
    "torch==2.14.1+cpu" && \
    pip install --no-cache-dir -r requirements.txt

# Bake the embedder into the image so runtime needs no host cache or Hub access.
ENV HF_HOME=/app/hf-cache \
    HF_HUB_OFFLINE=0
RUN python - <<'PY'
from sentence_transformers import SentenceTransformer

SentenceTransformer("sentence-transformers/all-MiniLM-L6-v2")
PY
ENV HF_HUB_OFFLINE=1

COPY . .
EXPOSE 8001
ENV DB_HOST=postgres \
    DB_PORT=5432 \
    DB_USER=arxiv \
    DB_PASSWORD=arxiv_secret \
    DB_NAME=arxiv_db \
    QDRANT_URL=http://qdrant:6333 \
    OLLAMA_HOST=http://ollama:11434 \
    LLM_PROVIDER=ollama

# python:slim ships no curl/wget — probe with urllib.
HEALTHCHECK --interval=10s --timeout=5s --start-period=120s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8001/health', timeout=5)" || exit 1
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8001"]
