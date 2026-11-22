"""Локальная проверка схем через Rust-ядро (C ABI), без сервера."""

from __future__ import annotations

import ctypes
import json
from pathlib import Path
from typing import Any

VERSION = "0.1.0"


def _library_path() -> Path:
    root = Path(__file__).resolve().parents[2]
    candidates = [
        root / "rust" / "target" / "release" / "libschemar_core.so",
        root / "rust" / "target" / "release" / "libschemar_core.dylib",
        root / "rust" / "target" / "release" / "schemar_core.dll",
    ]
    for candidate in candidates:
        if candidate.exists():
            return candidate
    raise FileNotFoundError("Rust core not built: run cargo build --release --manifest-path rust/Cargo.toml")


def _lib() -> ctypes.CDLL:
    library = ctypes.CDLL(str(_library_path()))
    library.schemar_check.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
    library.schemar_check.restype = ctypes.c_void_p
    library.schemar_fingerprint.argtypes = [ctypes.c_char_p]
    library.schemar_fingerprint.restype = ctypes.c_void_p
    library.schemar_free.argtypes = [ctypes.c_void_p]
    library.schemar_free.restype = None
    return library


def _call(function_name: str, *payloads: dict[str, Any]) -> dict[str, Any]:
    library = _lib()
    encoded = [json.dumps(payload, ensure_ascii=False).encode("utf-8") for payload in payloads]
    pointer = getattr(library, function_name)(*encoded)
    if not pointer:
        raise RuntimeError(f"{function_name} returned null")
    try:
        text = ctypes.string_at(pointer).decode("utf-8")
    finally:
        library.schemar_free(pointer)
    result = json.loads(text)
    if "error" in result:
        raise ValueError(result["error"])
    return result


def check(old: dict[str, Any], new: dict[str, Any]) -> dict[str, Any]:
    """Сравнивает две схемы локально, через Rust."""
    return _call("schemar_check", old, new)


def fingerprint(schema: dict[str, Any]) -> str:
    """Считает стабильный отпечаток схемы."""
    return _call("schemar_fingerprint", schema)["fingerprint"]
