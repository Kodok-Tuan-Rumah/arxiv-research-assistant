"""T8: validasi payload /ingest — id salah, >50 ids, overlap >= chunk_size → 422."""
from fastapi import FastAPI
from fastapi.testclient import TestClient

from routers.ingest import IngestRequest, router

app = FastAPI()
app.include_router(router)
client = TestClient(app)


def test_invalid_arxiv_id_returns_422():
    resp = client.post("/ingest", json={"arxiv_ids": ["not-an-arxiv-id"]})

    assert resp.status_code == 422


def test_too_many_arxiv_ids_returns_422():
    resp = client.post("/ingest", json={"arxiv_ids": [f"2401.{i:05d}" for i in range(51)]})

    assert resp.status_code == 422


def test_overlap_not_less_than_chunk_size_returns_422():
    resp = client.post(
        "/ingest",
        json={"arxiv_ids": ["2312.10997"], "chunk_size": 100, "overlap": 100},
    )

    assert resp.status_code == 422


def test_versioned_and_unversioned_ids_dedupe():
    request = IngestRequest(arxiv_ids=["2312.10997v1", "2312.10997"])

    assert request.arxiv_ids == ["2312.10997"]


def test_valid_payload_passes_validation():
    # Payload valid menembus validasi dan berhenti di guard model (tanpa lifespan
    # state.model belum terisi) — membuktikan 422 di atas benar-benar soal validasi.
    resp = client.post("/ingest", json={"arxiv_ids": ["2312.10997"]})

    assert resp.status_code == 503
    assert resp.json()["detail"] == "model not loaded"
