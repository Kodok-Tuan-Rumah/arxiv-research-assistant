"""T5: urutan PG-sebelum-Qdrant + kompensasi saat Qdrant gagal."""
from types import SimpleNamespace

import routers.ingest as ingest_mod
from engine.retrieval.qdrant_client import compute_point_ids

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
    """Stub semua dependency ingest() dan rekam urutan pemanggilan."""

    def __init__(self, upsert_error=None, parse_error=None, delete_paper_error=None):
        self.calls = []
        self.conn = FakeConn(self.calls)
        self.upsert_error = upsert_error
        self.parse_error = parse_error
        self.delete_paper_error = delete_paper_error
        self.inserted_ids = None

    def get_connection(self):
        self.calls.append("get_connection")
        return self.conn

    def upsert_paper(self, conn, paper):
        self.calls.append("upsert_paper")
        return PAPER_UUID

    def insert_chunks(self, conn, paper_id, chunks, ids):
        self.calls.append("insert_chunks")
        self.inserted_ids = list(ids)

    def delete_paper(self, conn, paper_id):
        self.calls.append("delete_paper")
        if self.delete_paper_error:
            raise self.delete_paper_error

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
        if self.parse_error:
            raise self.parse_error
        return "full text"

    def chunk_text(self, text, chunk_size, overlap):
        self.calls.append("chunk_text")
        return ["chunk a", "chunk b", "chunk c"]

    def get_client(self):
        return SimpleNamespace()

    def ensure_collection(self, client):
        self.calls.append("ensure_collection")

    def delete_by_arxiv_id(self, client, arxiv_id):
        self.calls.append("qdrant_delete")

    def upsert_chunks(self, client, chunks, ids=None):
        self.calls.append("qdrant_upsert")
        assert ids is not None
        if self.upsert_error:
            raise self.upsert_error
        return ids

    def install(self, monkeypatch):
        monkeypatch.setattr(ingest_mod.app_state, "model", FakeModel())
        monkeypatch.setattr(ingest_mod, "get_connection", self.get_connection)
        monkeypatch.setattr(ingest_mod, "upsert_paper", self.upsert_paper)
        monkeypatch.setattr(ingest_mod, "insert_chunks", self.insert_chunks)
        monkeypatch.setattr(ingest_mod, "delete_paper", self.delete_paper)
        monkeypatch.setattr(ingest_mod, "fetch_metadata", self.fetch_metadata)
        monkeypatch.setattr(ingest_mod, "download_and_parse_pdf", self.download_and_parse_pdf)
        monkeypatch.setattr(ingest_mod, "chunk_text", self.chunk_text)
        monkeypatch.setattr(ingest_mod, "get_client", self.get_client)
        monkeypatch.setattr(ingest_mod, "ensure_collection", self.ensure_collection)
        monkeypatch.setattr(ingest_mod, "delete_by_arxiv_id", self.delete_by_arxiv_id)
        monkeypatch.setattr(ingest_mod, "upsert_chunks", self.upsert_chunks)


def test_success_commits_pg_before_touching_qdrant(monkeypatch):
    h = Harness()
    h.install(monkeypatch)

    result = ingest_mod.ingest(ingest_mod.IngestRequest(arxiv_ids=[ARXIV_ID]))

    assert result.ingested == [ARXIV_ID]
    assert result.failed == []
    assert result.total_chunks == 3

    order = h.calls
    assert order.index("insert_chunks") < order.index("pg_commit")
    assert order.index("pg_commit") < order.index("qdrant_delete")
    assert order.index("qdrant_delete") < order.index("qdrant_upsert")
    assert h.inserted_ids == compute_point_ids(ARXIV_ID, 3)


def test_qdrant_failure_after_pg_commit_compensates(monkeypatch):
    h = Harness(upsert_error=RuntimeError("qdrant down"))
    h.install(monkeypatch)

    result = ingest_mod.ingest(ingest_mod.IngestRequest(arxiv_ids=[ARXIV_ID]))

    assert result.ingested == []
    assert result.failed == [ARXIV_ID]
    assert result.total_chunks == 0

    order = h.calls
    assert order.index("pg_commit") < order.index("qdrant_upsert") < order.index("delete_paper")
    assert order.count("pg_commit") == 2  # komit awal + komit kompensasi
    assert order[-2:] == ["delete_paper", "pg_commit"]


def test_failure_before_pg_commit_rolls_back(monkeypatch):
    h = Harness(parse_error=RuntimeError("pdf unavailable"))
    h.install(monkeypatch)

    result = ingest_mod.ingest(ingest_mod.IngestRequest(arxiv_ids=[ARXIV_ID]))

    assert result.failed == [ARXIV_ID]
    assert "pg_rollback" in h.calls
    assert "delete_paper" not in h.calls
    assert "qdrant_upsert" not in h.calls
    assert "insert_chunks" not in h.calls


def test_compensation_failure_is_contained(monkeypatch):
    h = Harness(
        upsert_error=RuntimeError("qdrant down"),
        delete_paper_error=RuntimeError("db also down"),
    )
    h.install(monkeypatch)

    result = ingest_mod.ingest(ingest_mod.IngestRequest(arxiv_ids=[ARXIV_ID]))

    assert result.failed == [ARXIV_ID]
    assert h.calls.count("pg_commit") == 1
    assert "pg_rollback" in h.calls
