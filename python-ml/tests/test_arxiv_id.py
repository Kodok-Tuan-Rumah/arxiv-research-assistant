import pytest
from engine.arxiv_id import is_valid_arxiv_id, normalize_arxiv_id


def test_normalize_strips_version_suffix():
    assert normalize_arxiv_id("2312.10997v1") == "2312.10997"
    assert normalize_arxiv_id("2312.10997v12") == "2312.10997"
    assert normalize_arxiv_id("2312.10997") == "2312.10997"


def test_normalize_is_idempotent():
    once = normalize_arxiv_id("2312.10997v3")
    assert normalize_arxiv_id(once) == once


@pytest.mark.parametrize(
    "value", ["", "   ", "not-an-arxiv-id", "2312.109", "2312.109977"]
)
def test_normalize_rejects_empty_and_invalid(value):
    with pytest.raises(ValueError):
        normalize_arxiv_id(value)


def test_is_valid_arxiv_id():
    assert is_valid_arxiv_id("2312.10997") is True
    assert is_valid_arxiv_id("2401.00001") is True
    assert is_valid_arxiv_id("2312.10997v1") is False
    assert is_valid_arxiv_id("arXiv:2312.10997") is False
    assert is_valid_arxiv_id("") is False
