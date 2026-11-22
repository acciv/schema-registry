"""HTTP-клиент для Go-сервера schema-registry."""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from typing import Any


class RegistryError(Exception):
    def __init__(self, status: int, message: str, body: str = ""):
        super().__init__(message)
        self.status = status
        self.body = body


class Client:
    def __init__(self, base_url: str = "http://127.0.0.1:8080", timeout: float = 5.0):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    def _request(self, method: str, path: str, payload: dict[str, Any] | None = None) -> Any:
        data = None
        headers = {"Accept": "application/json"}
        if payload is not None:
            data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(self.base_url + path, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                raw = response.read().decode("utf-8")
                status = response.status
        except urllib.error.HTTPError as error:
            raw = error.read().decode("utf-8", "replace")
            status = error.code
            error.close()
        except urllib.error.URLError as error:
            raise RegistryError(0, str(error.reason)) from error

        try:
            decoded = json.loads(raw) if raw else {}
        except ValueError:
            decoded = {"raw": raw}

        if status >= 400:
            message = decoded.get("error", f"HTTP {status}") if isinstance(decoded, dict) else f"HTTP {status}"
            raise RegistryError(status, str(message), raw)
        return decoded

    def health(self) -> dict[str, Any]:
        return self._request("GET", "/healthz")

    def stats(self) -> dict[str, Any]:
        return self._request("GET", "/stats")

    def list_schemas(self) -> list[str]:
        return self._request("GET", "/v1/schemas").get("names", [])

    def versions(self, name: str) -> list[dict[str, Any]]:
        return self._request("GET", f"/v1/schemas/{name}").get("versions", [])

    def get(self, name: str, version: str) -> dict[str, Any]:
        return self._request("GET", f"/v1/schemas/{name}/{version}")

    def push(
        self,
        name: str,
        version: str,
        schema: dict[str, Any],
        allow_breaking: bool = False,
    ) -> dict[str, Any]:
        return self._request(
            "POST",
            "/v1/schemas",
            {"name": name, "version": version, "schema": schema, "allow_breaking": allow_breaking},
        )

    def check(self, old: dict[str, Any], new: dict[str, Any]) -> dict[str, Any]:
        return self._request("POST", "/v1/check", {"old": old, "new": new})

    def check_version(self, name: str, version: str, schema: dict[str, Any]) -> dict[str, Any]:
        return self._request("POST", f"/v1/schemas/{name}/{version}/check", {"schema": schema})
