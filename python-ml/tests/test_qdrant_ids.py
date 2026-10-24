from types import SimpleNamespace

import pytest
from qdrant_client.models import FilterSelector, PointIdsList

from engine.retrieval.qdrant_client import (
    COLLECTION_NAME,
    compute_point_ids,
    delete_by_arxiv_id,
    delete_stale_chunks,
    restore_snapshot,
    search,
    snapshot_by_arxiv_id,
    upsert_chunks,
)


class FakeQdrant:
    def __init__(self):
        self.upserts = []
        self.deletes = []
        self.searches = []
        self.scrolls = []
        self.scroll_result: list = []

    def upsert(self, collection_name, points):
        self.upserts.append((collection_name, points))

    def delete(self, collection_name, points_selector):
        self.deletes.append((collection_name, points_selector))

    def query_points(self, collection_name, query, limit, query_filter):
        self.searches.append((collection_name, query, limit, query_filter))
        return SimpleNamespace(points=[])

    def scroll(self, collection_name, scroll_filter=None, limit=10, offset=None,
               with_vectors=False, with_payload=False):
        self.scrolls.append((collection_name, scroll_filter, with_vectors, with_payload))
        return self.scroll_result, None


def _chunks(count, arxiv_id="2312.10997"):
    return [
        {
            "text": f"chunk {i}",
            "embedding": [0.0, 1.0],
            "arxiv_id": arxiv_id,
            "categories": ["cs.CL"],
        }
        for i in range(count)
    ]


def test_compute_point_ids_is_deterministic():
    first = compute_point_ids("2312.10997", 5)
    second = compute_point_ids("2312.10997", 5)
    other = compute_point_ids("2401.00001", 5)

    assert first == second
    assert first != other
    assert len(set(first)) == 5


def test_upsert_chunks_uses_deterministic_ids():
    client = FakeQdrant()

    ids_first = upsert_chunks(client, _chunks(3))
    ids_second = upsert_chunks(client, _chunks(3))

    assert ids_first == ids_second == compute_point_ids("2312.10997", 3)
    assert len(client.upserts) == 2


def test_upsert_chunks_accepts_explicit_ids():
    client = FakeQdrant()
    ids = compute_point_ids("2312.10997", 2)

    returned = upsert_chunks(client, _chunks(2), ids=ids)

    assert returned == ids
    assert [point.id for point in client.upserts[0][1]] == ids


def test_upsert_chunks_rejects_mismatched_ids():
    client = FakeQdrant()

    with pytest.raises(ValueError):
        upsert_chunks(client, _chunks(2), ids=["only-one"])


def test_delete_by_arxiv_id_targets_payload_filter():
    client = FakeQdrant()

    delete_by_arxiv_id(client, "2312.10997")

    assert len(client.deletes) == 1
    collection, selector = client.deletes[0]
    assert collection == COLLECTION_NAME
    assert isinstance(selector, FilterSelector)
    condition = selector.filter.must[0]
    assert condition.key == "arxiv_id"
    assert condition.match.value == "2312.10997"


def test_delete_normalizes_versioned_id():
    client = FakeQdrant()

    delete_by_arxiv_id(client, "2312.10997v3")

    condition = client.deletes[0][1].filter.must[0]
    assert condition.match.value == "2312.10997"


def test_search_filter_normalizes_versioned_id():
    client = FakeQdrant()

    search(client, [0.0, 1.0], top_k=3, arxiv_id="2312.10997v2")

    _, _, _, query_filter = client.searches[0]
    condition = query_filter.must[0]
    assert condition.key == "arxiv_id"
    assert condition.match.value == "2312.10997"


def test_compute_point_ids_are_same_for_versioned_and_unversioned():
    assert compute_point_ids("2312.10997v4", 3) == compute_point_ids("2312.10997", 3)


def test_snapshot_normalizes_id_and_converts_points():
    client = FakeQdrant()
    client.scroll_result = [
        SimpleNamespace(id="p1", vector=[0.0, 1.0], payload={"arxiv_id": "2312.10997v2", "text": "t"}),
        SimpleNamespace(id="p2", vector=[1.0, 0.0], payload=None),
    ]

    snapshot = snapshot_by_arxiv_id(client, "2312.10997v2")

    assert [point["id"] for point in snapshot] == ["p1", "p2"]
    assert snapshot[1]["vector"] == [1.0, 0.0]
    assert snapshot[1]["payload"] == {}
    collection, scroll_filter, with_vectors, with_payload = client.scrolls[0]
    assert collection == COLLECTION_NAME
    assert scroll_filter.must[0].match.value == "2312.10997"
    assert with_vectors is True and with_payload is True


def test_restore_snapshot_deletes_then_reupserts_snapshot_points():
    client = FakeQdrant()
    snap = [
        {"id": "p1", "vector": [0.0, 1.0], "payload": {"arxiv_id": "2312.10997"}},
        {"id": "p2", "vector": [1.0, 0.0], "payload": {}},
    ]

    restore_snapshot(client, "2312.10997", snap)

    restor_data = client.upserts[0]
    assert restor_data[0] == COLLECTION_NAME
    assert [point.id for point in restor_data[1]] == ["p1", "p2"]


def test_restore_snapshot_with_empty_snapshot_only_deletes():
    client = FakeQdrant()

    restore_snapshot(client, "2312.10997", [])

    assert client.upserts == []
    assert len(client.deletes) == 1


def test_delete_stale_chunks_keeps_active_ids():
    client = FakeQdrant()
    client.scroll_result = [
        SimpleNamespace(id="keep-1", vector=[0.0], payload={}),
        SimpleNamespace(id="stale-1", vector=[0.0], payload={}),
        SimpleNamespace(id="stale-2", vector=[0.0], payload={}),
    ]

    removed = delete_stale_chunks(client, "2312.10997", ["keep-1"])

    assert removed == 2
    _, selector = client.deletes[0]
    assert isinstance(selector, PointIdsList)
    assert sorted(selector.points) == ["stale-1", "stale-2"]


def test_delete_stale_chunks_without_stale_writes_nothing():
    client = FakeQdrant()
    client.scroll_result = [
        SimpleNamespace(id="keep-1", vector=[0.0], payload={}),
        SimpleNamespace(id="keep-2", vector=[0.0], payload={}),
    ]

    assert delete_stale_chunks(client, "2312.10997", ["keep-1", "keep-2"]) == 0
    assert client.deletes == []
