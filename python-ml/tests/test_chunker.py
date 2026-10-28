import pytest
from engine.ingestion.chunker import MAX_TEXT_CHARS, chunk_text


def test_chunk_text_basic():
    chunks = chunk_text("word " * 400, chunk_size=300, overlap=50)
    assert len(chunks) >= 2
    assert all(chunk.strip() for chunk in chunks)


def test_chunk_text_rejects_overlap_not_less_than_chunk_size():
    with pytest.raises(ValueError):
        chunk_text("some text", chunk_size=10, overlap=10)
    with pytest.raises(ValueError):
        chunk_text("some text", chunk_size=10, overlap=11)
    with pytest.raises(ValueError):
        chunk_text("some text", chunk_size=10, overlap=-1)


def test_chunk_text_rejects_non_positive_chunk_size():
    with pytest.raises(ValueError):
        chunk_text("some text", chunk_size=0, overlap=0)


def test_chunk_text_rejects_oversized_text():
    with pytest.raises(ValueError):
        chunk_text("a" * (MAX_TEXT_CHARS + 1), chunk_size=512, overlap=50)
