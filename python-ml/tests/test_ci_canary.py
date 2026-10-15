"""Canary T8: sengaja GAGAL, membuktikan gate CI benar-benar bisa merah.
File ini dihapus di commit berikutnya."""


def test_ci_gate_canary():
    raise AssertionError("canary: CI harus merah pada commit ini")
