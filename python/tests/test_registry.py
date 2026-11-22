import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from schemaregistry.client import Client, RegistryError
from schemaregistry.core import check, fingerprint

V1 = {"type": "object", "properties": {"id": {"type": "string"}}, "required": ["id"]}
V2 = {"type": "object", "properties": {"id": {"type": "string"}, "page": {"type": "integer"}}, "required": ["id"]}
V3 = {"type": "object", "properties": {"id": {"type": "string"}, "token": {"type": "string"}}, "required": ["id", "token"]}


class CoreTest(unittest.TestCase):
    def test_optional_change_is_compatible(self):
        self.assertTrue(check(V1, V2)["compatible"])

    def test_required_change_is_breaking(self):
        report = check(V1, V3)
        self.assertFalse(report["compatible"])
        self.assertGreaterEqual(report["breaking"], 1)

    def test_fingerprint_is_stable(self):
        self.assertEqual(fingerprint(V1), fingerprint(V1))
        self.assertNotEqual(fingerprint(V1), fingerprint(V2))


class StubHandler(BaseHTTPRequestHandler):
    def _send(self, status, payload):
        data = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/healthz":
            self._send(200, {"status": "ok", "core": "0.1.0"})
        elif self.path == "/v1/schemas":
            self._send(200, {"names": ["user.created"]})
        elif self.path.startswith("/v1/schemas/"):
            self._send(200, {"name": "user.created", "version": "1.0.0"})
        else:
            self._send(404, {"error": "not found"})

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length) or b"{}")
        if self.path == "/v1/schemas" and body.get("version") == "2.0.0":
            self._send(409, {"error": "breaking change rejected", "report": {"compatible": False}})
        elif self.path == "/v1/check":
            self._send(200, check(body["old"], body["new"]))
        else:
            self._send(201, {"entry": {"name": body.get("name"), "version": body.get("version")}})

    def log_message(self, *_args):
        pass


class ClientTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), StubHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.client = Client(f"http://127.0.0.1:{cls.server.server_port}")

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=2)

    def test_health(self):
        self.assertEqual(self.client.health()["status"], "ok")

    def test_list(self):
        self.assertEqual(self.client.list_schemas(), ["user.created"])

    def test_push_ok(self):
        result = self.client.push("user.created", "1.0.0", V1)
        self.assertEqual(result["entry"]["version"], "1.0.0")

    def test_push_conflict_raises(self):
        with self.assertRaises(RegistryError) as ctx:
            self.client.push("user.created", "2.0.0", V3)
        self.assertEqual(ctx.exception.status, 409)

    def test_check(self):
        report = self.client.check(V1, V2)
        self.assertTrue(report["compatible"])


if __name__ == "__main__":
    unittest.main()
