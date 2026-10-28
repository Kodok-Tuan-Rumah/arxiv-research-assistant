"""python-ml tidak boleh punya LLM judge sendiri.

`routers/evaluate.py` dan `engine/evaluation/faithfulness.py` menilai
faithfulness dengan meminta `phi3:mini` menjadi juri. Endpoint itu tidak pernah
dipanggil go-backend: ia hanya menambah jalur kedua yang bisa memberi skor
berbeda dari `llm-eval-framework` untuk jawaban yang sama.

Produk hanya punya satu evaluator, jadi keduanya dihapus. Test ini menjaga agar
jalur kedua tidak tumbuh kembali tanpa sengaja.
"""
from __future__ import annotations

import ast
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[1]


def test_legacy_evaluate_router_is_gone():
    assert not (ROOT / "routers" / "evaluate.py").exists()
    assert not (ROOT / "engine" / "evaluation" / "faithfulness.py").exists()


def test_main_does_not_import_evaluate_router():
    tree = ast.parse((ROOT / "main.py").read_text())
    imported = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom) and node.module == "routers":
            imported.update(alias.name for alias in node.names)
    assert "evaluate" not in imported


def test_main_does_not_mount_evaluate_router():
    tree = ast.parse((ROOT / "main.py").read_text())
    for node in ast.walk(tree):
        if (
            isinstance(node, ast.Attribute)
            and node.attr == "router"
            and isinstance(node.value, ast.Name)
            and node.value.id == "evaluate"
        ):
            raise AssertionError("main.py masih memasang evaluate.router")
