import logging
import os
import tempfile

import fitz  # pymupdf
import httpx

logger = logging.getLogger(__name__)

MAX_PDF_BYTES = 50 * 1024 * 1024
MIN_EXTRACTED_CHARS = 50


class PdfParseError(ValueError):
    """A PDF could not be turned into ingestable text (carries an error code)."""

    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code


def extract_pdf_text(tmp_path: str) -> str:
    """Convert an openable PDF file into text, refusing empty/short extractions."""
    doc = fitz.open(tmp_path)
    try:
        text = ""
        for page in doc:
            text += page.get_text()
    finally:
        doc.close()

    if not text.strip():
        raise PdfParseError("no_extractable_text", "PDF yielded no extractable text")
    stripped = text.strip()
    if len(stripped) < MIN_EXTRACTED_CHARS:
        raise PdfParseError(
            "extracted_text_too_short",
            f"PDF yielded only {len(stripped)} characters",
        )
    logger.info(f"Parsed PDF: {len(text)} chars extracted")
    return text


def download_and_parse_pdf(pdf_url: str) -> str:
    logger.info(f"Downloading PDF: {pdf_url}")

    with (
        httpx.Client(timeout=60, follow_redirects=True) as client,
        client.stream("GET", pdf_url) as response,
    ):
        response.raise_for_status()

        content_length = response.headers.get("content-length")
        if (
            content_length
            and content_length.isdigit()
            and int(content_length) > MAX_PDF_BYTES
        ):
            raise ValueError(f"PDF too large: {content_length} bytes (max {MAX_PDF_BYTES})")

        data = bytearray()
        for part in response.iter_bytes(chunk_size=65536):
            data.extend(part)
            if len(data) > MAX_PDF_BYTES:
                raise ValueError(f"PDF exceeds size cap ({MAX_PDF_BYTES} bytes)")

    with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False) as f:
        f.write(bytes(data))
        tmp_path = f.name

    try:
        return extract_pdf_text(tmp_path)
    finally:
        os.unlink(tmp_path)
