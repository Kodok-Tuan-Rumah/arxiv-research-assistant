import logging
import os

logger = logging.getLogger(__name__)

model = None

def load_model():
    global model
    # Skip if running in cloud mode (no local embedding model needed)
    if os.getenv("CLOUD_MODE", "false").lower() == "true":
        logger.info("CLOUD_MODE=true, skipping local embedding model")
        return
    try:
        from sentence_transformers import SentenceTransformer
        model = SentenceTransformer("all-MiniLM-L6-v2")
        logger.info("Embedding model loaded")
    except ImportError:
        logger.warning("sentence_transformers not available, embedding disabled")
