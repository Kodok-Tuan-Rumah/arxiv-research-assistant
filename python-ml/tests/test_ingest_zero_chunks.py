import tempfile
from types import SimpleNamespace

import fitz
import pytest
import routers.ingest as ingest_mod
from engine.ingestion.pdf_parser import (
    MIN_EXTRACTED_CHARS,
    PdfParseError,
    extract_pdf_text,
)

ARXIV_ID = "2312.10997"
PAPER_UUID = "11111111-2222-3333-4444-555555555555"


class FakeModel:
    def encode(self, texts):
        return SimpleNamespace(tolist=lambda: [[0.0] * 8 for _ in texts])


class FakeConn:
    def __init__(self, calls):
        self.calls = calls

    def commit(self):
        self.calls.append("pg_commit")

    def rollback(self):
        self.calls.append("pg_rollback")

    def close(self):
        pass


class Harness:
    def __init__(self, parsed_text="full text " * 10, chunk_result=None):
        self.calls = []
        self.conn = FakeConn(self.calls)
        self.parsed_text = parsed_text
        self.chunk_result = chunk_result

    def get_connection(self):
        self.calls.append("get_connection")
        return self.conn

    def upsert_paper(self, conn, paper):
        self.calls.append("upsert_paper")
        return PAPER_UUID

    def insert_chunks(self, conn, paper_id, chunks, ids):
        self.calls.append("insert_chunks")

    def fetch_metadata(self, arxiv_ids):
        self.calls.append("fetch_metadata")
        return [
            SimpleNamespace(
                arxiv_id=ARXIV_ID,
                pdf_url="http://example.test/paper.pdf",
                categories=["cs.CL"],
            )
        ]

    def download_and_parse_pdf(self, pdf_url):
        self.calls.append("parse_pdf")
        return self.parsed_text

    def chunk_text(self, text, chunk_size, overlap):
        self.calls.append("chunk_text")
        if self.chunk_result is not None:
            return list(self.chunk_result)
        return [text.strip()] if text.strip() else []

    def get_client(self):
        return SimpleNamespace()

    def ensure_collection(self, client):
        self.calls.append("ensure_collection")

    def snapshot_by_arxiv_id(self, client, arxiv_id):
        self.calls.append("qdrant_snapshot")
        return [
            {"id": "old-point-1", "vector": [0.0], "payload": {"arxiv_id": arxiv_id}}
        ]

    def restore_snapshot(self, client, arxiv_id, snapshot):
        self.calls.append("qdrant_restore")

    def delete_stale_chunks(self, client, arxiv_id, keep_ids):
        self.calls.append("qdrant_delete_stale")
        return 0

    def upsert_chunks(self, client, chunks, ids=None):
        self.calls.append("qdrant_upsert")
        return ids

    def install(self, monkeypatch):
        monkeypatch.setattr(ingest_mod.app_state, "model", FakeModel())
        monkeypatch.setattr(ingest_mod, "get_connection", self.get_connection)
        monkeypatch.setattr(ingest_mod, "upsert_paper", self.upsert_paper)
        monkeypatch.setattr(ingest_mod, "insert_chunks", self.insert_chunks)
        monkeypatch.setattr(ingest_mod, "fetch_metadata", self.fetch_metadata)
        monkeypatch.setattr(
            ingest_mod, "download_and_parse_pdf", self.download_and_parse_pdf
        )
        monkeypatch.setattr(ingest_mod, "chunk_text", self.chunk_text)
        monkeypatch.setattr(ingest_mod, "get_client", self.get_client)
        monkeypatch.setattr(ingest_mod, "ensure_collection", self.ensure_collection)
        monkeypatch.setattr(
            ingest_mod, "snapshot_by_arxiv_id", self.snapshot_by_arxiv_id
        )
        monkeypatch.setattr(ingest_mod, "restore_snapshot", self.restore_snapshot)
        monkeypatch.setattr(ingest_mod, "delete_stale_chunks", self.delete_stale_chunks)
        monkeypatch.setattr(ingest_mod, "upsert_chunks", self.upsert_chunks)


def _run(h, monkeypatch):
    h.install(monkeypatch)
    return ingest_mod.ingest(ingest_mod.IngestRequest(arxiv_ids=[ARXIV_ID]))


def test_empty_pdf_is_rejected_before_pg_or_qdrant(monkeypatch):
    h = Harness(parsed_text="")
    result = _run(h, monkeypatch)

    assert result.failed == [ARXIV_ID]
    assert result.ingested == []
    assert "upsert_paper" not in h.calls
    assert "insert_chunks" not in h.calls
    assert "pg_commit" not in h.calls
    assert "qdrant_snapshot" not in h.calls
    assert "qdrant_restore" not in h.calls
    assert "qdrant_delete_stale" not in h.calls
    assert "qdrant_upsert" not in h.calls


def test_empty_chunk_list_is_rejected(monkeypatch):
    h = Harness(parsed_text="a" * 600, chunk_result=[])
    result = _run(h, monkeypatch)

    assert result.failed == [ARXIV_ID]
    assert "qdrant_upsert" not in h.calls
    assert "upsert_paper" not in h.calls


def test_valid_paper_still_ingests(monkeypatch):
    h = Harness(parsed_text="a" * 600)
    result = _run(h, monkeypatch)

    assert result.ingested == [ARXIV_ID]
    assert result.failed == []
    assert "pg_commit" in h.calls
    assert "qdrant_upsert" in h.calls


def _write_pdf(text: str) -> str:
    doc = fitz.open()
    page = doc.new_page()
    if text:
        page.insert_text((72, 72), text, fontsize=11)
    tmp_path = tempfile.mkstemp(suffix=".pdf")[1]
    doc.save(tmp_path)
    doc.close()
    return tmp_path


def test_blank_pdf_is_rejected():
    path = _write_pdf("")
    with pytest.raises(PdfParseError) as excinfo:
        extract_pdf_text(path)
    assert excinfo.value.code == "no_extractable_text"


def test_short_pdf_is_rejected():
    path = _write_pdf("che miti lengkung")
    with pytest.raises(PdfParseError) as excinfo:
        extract_pdf_text(path)
    assert excinfo.value.code == "extracted_text_too_short"
    assert "only" in str(excinfo.value)


def test_long_enough_pdf_is_accepted():
    path = _write_pdf("a" * (MIN_EXTRACTED_CHARS + 5))
    text = extract_pdf_text(path)
    assert text.startswith("a")
    assert len(text) >= MIN_EXTRACTED_CHARS


def test_parser_minimum_chars():
    assert MIN_EXTRACTED_CHARS == 50
