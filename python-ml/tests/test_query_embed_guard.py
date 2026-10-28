import pytest
import routers.query as query_mod
import state
from fastapi import HTTPException


class _Vector:
    def tolist(self):
        return [0.5] * 4


class _FakeModel:
    def encode(self, texts):
        return [_Vector()]


def test_embed_query_requires_model(monkeypatch):
    monkeypatch.setattr(state, "model", None)
    with pytest.raises(HTTPException) as exc_info:
        query_mod.embed_query("hello")
    assert exc_info.value.status_code == 503
    assert exc_info.value.detail == "embedding model not loaded"


def test_embed_query_uses_loaded_model(monkeypatch):
    monkeypatch.setattr(state, "model", _FakeModel())
    assert query_mod.embed_query("hello") == [0.5] * 4
