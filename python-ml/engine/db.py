import logging
import os

import psycopg2
import psycopg2.extras

from engine.arxiv_id import normalize_arxiv_id

logger = logging.getLogger(__name__)

def get_connection():
    return psycopg2.connect(
        host=os.getenv("DB_HOST", "localhost"),
        port=os.getenv("DB_PORT", "5432"),
        user=os.getenv("DB_USER", "arxiv"),
        password=os.getenv("DB_PASSWORD", "arxiv_secret"),
        dbname=os.getenv("DB_NAME", "arxiv_db"),
    )

def upsert_paper(conn, paper) -> str:
    """Insert paper, return paper UUID. Skip if arxiv_id already exists."""
    arxiv_id = normalize_arxiv_id(paper.arxiv_id)
    with conn.cursor() as cur:
        cur.execute("""
            INSERT INTO papers (arxiv_id, title, authors, abstract, categories, published)
            VALUES (%s, %s, %s, %s, %s, %s)
            ON CONFLICT (arxiv_id) DO UPDATE SET
                title = EXCLUDED.title,
                authors = EXCLUDED.authors,
                abstract = EXCLUDED.abstract,
                categories = EXCLUDED.categories,
                published = EXCLUDED.published
            RETURNING id
        """, (
            arxiv_id,
            paper.title,
            paper.authors,
            paper.abstract,
            paper.categories,
            paper.published,
        ))
        return str(cur.fetchone()[0])

def insert_chunks(conn, paper_id: str, chunks: list[str], qdrant_ids: list[str]):
    """Insert chunks for a paper. Delete existing first to avoid duplicates."""
    with conn.cursor() as cur:
        cur.execute("DELETE FROM chunks WHERE paper_id = %s", (paper_id,))
        psycopg2.extras.execute_batch(cur, """
            INSERT INTO chunks (paper_id, chunk_index, text, qdrant_id, char_count)
            VALUES (%s, %s, %s, %s, %s)
        """, [
            (paper_id, i, chunk, qdrant_ids[i] if i < len(qdrant_ids) else None, len(chunk))
            for i, chunk in enumerate(chunks)
        ])

def delete_paper(conn, paper_id: str) -> None:
    """Delete a paper row (chunks cascade via FK). Used to compensate a failed
    Qdrant write after the PostgreSQL commit."""
    with conn.cursor() as cur:
        cur.execute("DELETE FROM papers WHERE id = %s", (paper_id,))
