from engine.retrieval import qdrant_client


def test_get_client_passes_api_key_for_remote_qdrant(monkeypatch):
    monkeypatch.setenv("QDRANT_URL", "http://qdrant:6333")
    monkeypatch.setenv("QDRANT_API_KEY", "test-qdrant-key")
    captured = {}

    def fake_client(**kwargs):
        captured.update(kwargs)
        return object()

    monkeypatch.setattr(qdrant_client, "QdrantClient", fake_client)

    qdrant_client.get_client()

    assert captured == {
        "url": "http://qdrant:6333",
        "api_key": "test-qdrant-key",
    }
