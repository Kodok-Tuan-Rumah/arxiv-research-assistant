import logging
import os
import tempfile

import fitz  # pymupdf
import httpx

logger = logging.getLogger(__name__)

def download_and_parse_pdf(pdf_url: str) -> str:
    logger.info(f"Downloading PDF: {pdf_url}")
    
    with httpx.Client(timeout=60, follow_redirects=True) as client:
        response = client.get(pdf_url)
        response.raise_for_status()
    
    with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False) as f:
        f.write(response.content)
        tmp_path = f.name
    
    try:
        doc = fitz.open(tmp_path)
        text = ""
        for page in doc:
            text += page.get_text()
        doc.close()
        logger.info(f"Parsed PDF: {len(text)} chars extracted")
        return text
    finally:
        os.unlink(tmp_path)
