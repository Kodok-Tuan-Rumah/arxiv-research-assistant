import logging
import os
import uuid

from engine.arxiv_id import normalize_arxiv_id
from qdrant_client import QdrantClient
from qdrant_client.models import Distance, PointStruct, VectorParams

logger = logging.getLogger(__name__)

COLLECTION_NAME = "arxiv_chunks"
VECTOR_SIZE = 384


def get_client(host: str | None = None, port: int | None = None) -> QdrantClient:
    url = os.getenv("QDRANT_URL")
    if url:
        return QdrantClient(url=url)
    return QdrantClient(host=host or "localhost", port=port or 6333)


def ensure_collection(client: QdrantClient):
    collections = [c.name for c in client.get_collections().collections]
    if COLLECTION_NAME not in collections:
        client.create_collection(
            collection_name=COLLECTION_NAME,
            vectors_config=VectorParams(size=VECTOR_SIZE, distance=Distance.COSINE),
        )
        logger.info(f"Created collection: {COLLECTION_NAME}")
    else:
        logger.info(f"Collection already exists: {COLLECTION_NAME}")


def compute_point_ids(arxiv_id: str, count: int) -> list[str]:
    arxiv_id = normalize_arxiv_id(arxiv_id)
    return [
        str(uuid.uuid5(uuid.NAMESPACE_URL, f"{arxiv_id}:{i}")) for i in range(count)
    ]


def upsert_chunks(
    client: QdrantClient, chunks: list[dict], ids: list[str] | None = None
) -> list[str]:
    if not chunks:
        return []

    if ids is None:
        ids = compute_point_ids(chunks[0].get("arxiv_id", ""), len(chunks))
    elif len(ids) != len(chunks):
        raise ValueError("ids must have the same length as chunks")

    points = [
        PointStruct(
            id=ids[i],
            vector=chunk["embedding"],
            payload={
                "chunk_id": chunk.get("chunk_id", ""),
                "paper_id": chunk.get("paper_id", ""),
                "arxiv_id": chunk.get("arxiv_id", ""),
                "text": chunk["text"],
                "categories": chunk.get("categories", []),
            },
        )
        for i, chunk in enumerate(chunks)
    ]

    client.upsert(collection_name=COLLECTION_NAME, points=points)
    logger.info(f"Upserted {len(points)} chunks to Qdrant")
    return ids


def delete_by_arxiv_id(client: QdrantClient, arxiv_id: str) -> None:
    from qdrant_client.models import FieldCondition, Filter, FilterSelector, MatchValue

    arxiv_id = normalize_arxiv_id(arxiv_id)
    client.delete(
        collection_name=COLLECTION_NAME,
        points_selector=FilterSelector(
            filter=Filter(
                must=[FieldCondition(key="arxiv_id", match=MatchValue(value=arxiv_id))]
            )
        ),
    )
    logger.info(f"Deleted stale vectors for {arxiv_id}")


def snapshot_by_arxiv_id(client: QdrantClient, arxiv_id: str) -> list[dict]:
    from qdrant_client.models import FieldCondition, Filter, MatchValue

    arxiv_id = normalize_arxiv_id(arxiv_id)
    must = Filter(
        must=[FieldCondition(key="arxiv_id", match=MatchValue(value=arxiv_id))]
    )

    collected: list = []
    offset = None
    while True:
        batch, offset = client.scroll(
            collection_name=COLLECTION_NAME,
            scroll_filter=must,
            limit=256,
            offset=offset,
            with_vectors=True,
            with_payload=True,
        )
        collected.extend(batch)
        if offset is None:
            break

    snapshot = [
        {"id": point.id, "vector": point.vector, "payload": dict(point.payload or {})}
        for point in collected
    ]
    logger.info(f"Snapshotted {len(snapshot)} vector(s) for {arxiv_id}")
    return snapshot


def restore_snapshot(client: QdrantClient, arxiv_id: str, snapshot: list[dict]) -> None:
    from qdrant_client.models import PointStruct

    arxiv_id = normalize_arxiv_id(arxiv_id)
    delete_by_arxiv_id(client, arxiv_id)
    if snapshot:
        points = [
            PointStruct(
                id=point["id"],
                vector=point["vector"],
                payload=point.get("payload") or {},
            )
            for point in snapshot
        ]
        client.upsert(collection_name=COLLECTION_NAME, points=points)
    logger.info(f"Restored snapshot for {arxiv_id}: {len(snapshot)} vector(s)")


def delete_stale_chunks(
    client: QdrantClient, arxiv_id: str, keep_ids: list[str]
) -> int:
    from qdrant_client.models import PointIdsList

    arxiv_id = normalize_arxiv_id(arxiv_id)
    keep = set(keep_ids)
    stale = [
        point
        for point in snapshot_by_arxiv_id(client, arxiv_id)
        if point["id"] not in keep
    ]
    if stale:
        client.delete(
            collection_name=COLLECTION_NAME,
            points_selector=PointIdsList(points=[point["id"] for point in stale]),
        )
        logger.info(f"Deleted {len(stale)} stale vector(s) for {arxiv_id}")
    return len(stale)


def search(
    client: QdrantClient,
    query_vector: list[float],
    top_k: int = 5,
    arxiv_id: str | None = None,
):
    from qdrant_client.models import FieldCondition, Filter, MatchValue

    query_filter = None
    if arxiv_id:
        arxiv_id = normalize_arxiv_id(arxiv_id)
        query_filter = Filter(
            must=[FieldCondition(key="arxiv_id", match=MatchValue(value=arxiv_id))]
        )

    results = client.query_points(
        collection_name=COLLECTION_NAME,
        query=query_vector,
        limit=top_k,
        query_filter=query_filter,
    ).points

    return [
        {
            "text": r.payload["text"],
            "score": r.score,
            "arxiv_id": r.payload.get("arxiv_id"),
        }
        for r in results
    ]
