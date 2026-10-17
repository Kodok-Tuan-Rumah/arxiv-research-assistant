# syntax=docker/dockerfile:1
FROM python:3.12-slim
WORKDIR /app

# System dependencies for sentence-transformers / PyMuPDF
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r requirements.txt

COPY . .
EXPOSE 8001
ENV DB_HOST=postgres \
    DB_PORT=5432 \
    DB_USER=arxiv \
    DB_PASSWORD=arxiv_secret \
    DB_NAME=arxiv_db \
    ML_SERVICE_URL=http://python-ml:8001 \
    QDRANT_URL=http://qdrant:6333 \
    OLLAMA_HOST=http://ollama:11434 \
    LLM_PROVIDER=ollama
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=3 \
  CMD wget -qO- http://localhost:8001/health || exit 1
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8001"]
