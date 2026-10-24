"""A4: urutan ingest non-destruktif + kompensasi snapshot saat gagal."""
from types import SimpleNamespace

import routers.ingest as ingest_mod
from engine.retrieval.qdrant_client import compute_point_ids

ARXIV_ID = "2312.10997"
PAPER_UUID = "11111111-2222-3333-4444-555555555555"


class FakeModel:
    def encode(self, texts):
        return SimpleNamespace(tolist=lambda: [[0.0] * 8 for _ in texts])


class FakeConn:
    def __init__(self, calls, commit_error=None):
        self.calls = calls
        self.commit_error = commit_error

    def commit(self):
        self.calls.append("pg_commit")
        if self.commit_error:
            raise self.commit_error

    def rollback(self):
        self.calls.append("pg_rollback")

    def close(self):
        pass


class Harness:
    """Stub semua dependency ingest() dan rekam urutan pemanggilan."""

    def __init__(
        self,
        upsert_error=None,
        parse_error=None,
        restore_error=None,
        delete_stale_error=None,
        commit_error=None,
    ):
        self.calls = []
        self.conn = FakeConn(self.calls, commit_error=commit_error)
        self.upsert_error = upsert_error
        self.parse_error = parse_error
        self.restore_error = restore_error
        self.delete_stale_error = delete_stale_error
        self.inserted_ids = None
        self.snapshot = [{"id": "old-point-1", "vector": [0.0], "payload": {"arxiv_id": ARXIV_ID}}]

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
        # Tidak ada lagi alur yang boleh menghapus baris PG setelah komit.
        self.calls.append("delete_paper")

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

    def snapshot_by_arxiv_id(self, client, arxiv_id):
        self.calls.append("qdrant_snapshot")
        return self.snapshot

    def restore_snapshot(self, client, arxiv_id, snapshot):
        self.calls.append("qdrant_restore")
        if self.restore_error:
            raise self.restore_error

    def delete_stale_chunks(self, client, arxiv_id, keep_ids):
        self.calls.append("qdrant_delete_stale")
        if self.delete_stale_error:
            raise self.delete_stale_error
        return 0

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
        monkeypatch.setattr(ingest_mod, "fetch_metadata", self.fetch_metadata)
        monkeypatch.setattr(ingest_mod, "download_and_parse_pdf", self.download_and_parse_pdf)
        monkeypatch.setattr(ingest_mod, "chunk_text", self.chunk_text)
        monkeypatch.setattr(ingest_mod, "get_client", self.get_client)
        monkeypatch.setattr(ingest_mod, "ensure_collection", self.ensure_collection)
        monkeypatch.setattr(ingest_mod, "snapshot_by_arxiv_id", self.snapshot_by_arxiv_id)
        monkeypatch.setattr(ingest_mod, "restore_snapshot", self.restore_snapshot)
        monkeypatch.setattr(ingest_mod, "delete_stale_chunks", self.delete_stale_chunks)
        monkeypatch.setattr(ingest_mod, "upsert_chunks", self.upsert_chunks)

    def run(self, monkeypatch):
        self.install(monkeypatch)
        return ingest_mod.ingest(ingest_mod.IngestRequest(arxiv_ids=[ARXIV_ID]))


def test_success_snapshots_then_upserts_then_commits_then_deletes_stale(monkeypatch):
    h = Harness()
    result = h.run(monkeypatch)

    assert result.ingested == [ARXIV_ID]
    assert result.failed == []
    assert result.total_chunks == 3

    order = h.calls
    assert order.index("qdrant_snapshot") < order.index("qdrant_upsert")
    assert order.index("qdrant_upsert") < order.index("insert_chunks")
    assert order.index("insert_chunks") < order.index("pg_commit")
    assert order.index("pg_commit") < order.index("qdrant_delete_stale")
    assert "qdrant_restore" not in order
    assert "delete_paper" not in order
    assert h.inserted_ids == compute_point_ids(ARXIV_ID, 3)


def test_qdrant_failure_rolls_back_pg_and_restores_snapshot(monkeypatch):
    h = Harness(upsert_error=RuntimeError("qdrant down"))
    result = h.run(monkeypatch)

    assert result.ingested == []
    assert result.failed == [ARXIV_ID]
    assert result.total_chunks == 0

    order = h.calls
    assert "pg_commit" not in order
    assert "pg_rollback" in order
    assert "qdrant_restore" in order
    assert "qdrant_delete_stale" not in order
    assert "delete_paper" not in order
    assert order.index("qdrant_upsert") < order.index("qdrant_restore")


def test_pg_commit_failure_restores_snapshot(monkeypatch):
    h = Harness(commit_error=RuntimeError("db connection lost"))
    result = h.run(monkeypatch)

    assert result.ingested == []
    assert result.failed == [ARXIV_ID]

    order = h.calls
    assert "upsert_paper" in order
    assert "insert_chunks" in order
    assert order.count("pg_commit") == 1
    assert "pg_rollback" in order
    assert "qdrant_restore" in order
    assert order.index("pg_commit") < order.index("qdrant_restore")


def test_stale_delete_failure_does_not_fail_ingest(monkeypatch):
    h = Harness(delete_stale_error=RuntimeError("stale sweep failed"))
    result = h.run(monkeypatch)

    assert result.ingested == [ARXIV_ID]
    assert result.failed == []
    assert "qdrant_delete_stale" in h.calls
    assert h.calls.count("pg_commit") == 1


def test_restore_failure_is_contained(monkeypatch):
    h = Harness(upsert_error=RuntimeError("qdrant down"), restore_error=RuntimeError("also down"))
    result = h.run(monkeypatch)

    assert result.failed == [ARXIV_ID]
    assert "qdrant_restore" in h.calls
    assert "delete_paper" not in h.calls


def test_failure_before_qdrant_writes_rolls_back_without_restore(monkeypatch):
    h = Harness(parse_error=RuntimeError("pdf unavailable"))
    result = h.run(monkeypatch)

    assert result.failed == [ARXIV_ID]
    assert "pg_rollback" in h.calls
    assert "qdrant_snapshot" not in h.calls
    assert "qdrant_restore" not in h.calls
    assert "delete_paper" not in h.calls
    assert "qdrant_upsert" not in h.calls
    assert "insert_chunks" not in h.calls
