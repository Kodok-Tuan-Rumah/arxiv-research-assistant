import pytest
from qdrant_client.models import FilterSelector

from engine.retrieval.qdrant_client import (
    COLLECTION_NAME,
    compute_point_ids,
    delete_by_arxiv_id,
    upsert_chunks,
)


class FakeQdrant:
    def __init__(self):
        self.upserts = []
        self.deletes = []

    def upsert(self, collection_name, points):
        self.upserts.append((collection_name, points))

    def delete(self, collection_name, points_selector):
        self.deletes.append((collection_name, points_selector))


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
