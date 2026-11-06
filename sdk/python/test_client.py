"""Python SDK 单元测试（V2.4-02）。"""
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

from ydsz_trace import APIError, YdszClient


class _Handler(BaseHTTPRequestHandler):
    """可配置响应的模拟服务端。"""

    def log_message(self, *args):
        pass

    def _respond(self):
        handler = type(self)
        body = json.dumps(handler.body).encode("utf-8")
        self.send_response(handler.status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        type(self).last_path = self.path
        # 读取可能存在的请求体，避免连接状态异常
        length = int(self.headers.get("Content-Length") or 0)
        if length:
            self.rfile.read(length)
        self._respond()

    def do_POST(self):
        type(self).last_path = self.path
        length = int(self.headers.get("Content-Length") or 0)
        type(self).last_body = json.loads(self.rfile.read(length) or b"{}")
        self._respond()


def start_server(status=200, body=None):
    """启动模拟服务端。"""
    handler = type("H", (_Handler,), {"status": status, "body": body or {}})
    srv = HTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    return srv, handler


class YdszClientTest(unittest.TestCase):

    def setUp(self):
        self.srv, self.handler = start_server(
            200, {"code": 0, "message": "ok", "data": {"lines": ["a", "b"], "total": 2}}
        )
        self.port = self.srv.server_address[1]
        self.client = YdszClient(f"http://127.0.0.1:{self.port}", token="t", max_retries=0)

    def tearDown(self):
        self.srv.shutdown()

    def test_base_url_trimmed(self):
        c = YdszClient("http://x/", "")
        self.assertEqual(c.base_url, "http://x")

    def test_token_from_env(self):
        import os

        os.environ["YDSZ_TOKEN"] = "env-token"
        try:
            c = YdszClient("http://x")
            self.assertEqual(c.token, "env-token")
        finally:
            del os.environ["YDSZ_TOKEN"]

    def test_explicit_token_wins(self):
        import os

        os.environ["YDSZ_TOKEN"] = "env-token"
        try:
            c = YdszClient("http://x", token="explicit")
            self.assertEqual(c.token, "explicit")
        finally:
            del os.environ["YDSZ_TOKEN"]

    def test_search_logs(self):
        resp = self.client.search_logs(item=1, date="20261106", key="timeout")
        self.assertEqual(resp["lines"], ["a", "b"])
        self.assertEqual(resp["total"], 2)
        self.assertIn("/logs/search", self.handler.last_path)

    def test_search_logs_sends_body(self):
        self.client.search_logs(item=1, date="20261106", level="ERROR")
        self.assertEqual(self.handler.last_body["level"], "ERROR")
        self.assertEqual(self.handler.last_body["item"], 1)

    def test_get_context(self):
        self.handler.body = {
            "code": 0,
            "message": "ok",
            "data": {"before": ["a"], "match": "m", "after": ["b"]},
        }
        resp = self.client.get_context(42, ref_time="2026-11-06 10:00:00")
        self.assertEqual(resp["match"], "m")
        self.assertIn("/logs/context/42", self.handler.last_path)

    def test_nl2dsl_parse(self):
        self.handler.body = {
            "code": 0,
            "message": "ok",
            "data": {"level": "ERROR", "strategy": "rule", "confidence": 0.8},
        }
        resp = self.client.nl2dsl_parse("近3小时 ERROR")
        self.assertEqual(resp["level"], "ERROR")
        self.assertEqual(resp["strategy"], "rule")

    def test_list_templates(self):
        self.handler.body = {"code": 0, "message": "ok", "data": {"templates": [], "total": 0}}
        resp = self.client.list_templates("1h", 10)
        self.assertIn("templates", resp)
        self.assertIn("timeRange", self.handler.last_path)

    def test_get_anomalies(self):
        self.handler.body = {"code": 0, "message": "ok", "data": {"points": [], "total": 0}}
        resp = self.client.get_anomalies("24h", 20)
        self.assertIn("points", resp)

    def test_reason_incident(self):
        self.handler.body = {
            "code": 0,
            "message": "ok",
            "data": {"hypothesis": "连接池耗尽", "totalTokens": 120},
        }
        resp = self.client.reason_incident("服务超时")
        self.assertEqual(resp["hypothesis"], "连接池耗尽")

    def test_semantic_search(self):
        self.handler.body = {"code": 0, "message": "ok", "data": {"results": [], "degraded": False}}
        resp = self.client.semantic_search("timeout", "template", 5)
        self.assertIn("results", resp)
        self.assertIn("/api/v2/semantic/search", self.handler.last_path)

    def test_api_error_business_code(self):
        self.handler.status = 200
        self.handler.body = {"code": 403, "message": "无权限", "data": None, "traceId": "t1"}
        with self.assertRaises(APIError) as ctx:
            self.client.search_logs(item=1, date="20261106")
        self.assertEqual(ctx.exception.code, 403)
        self.assertEqual(ctx.exception.trace_id, "t1")

    def test_api_error_http_status(self):
        self.handler.status = 403
        self.handler.body = {"code": 403, "message": "forbidden", "data": None}
        with self.assertRaises(APIError):
            self.client.search_logs(item=1, date="20261106")

    def test_retry_then_success(self):
        calls = {"n": 0}

        class RetryHandler(_Handler):
            def do_POST(self):
                calls["n"] += 1
                if calls["n"] == 1:
                    body = b""
                    self.send_response(500)
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                body = json.dumps({"code": 0, "message": "ok", "data": {"ok": True}}).encode()
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        srv = HTTPServer(("127.0.0.1", 0), RetryHandler)
        thread = threading.Thread(target=srv.serve_forever, daemon=True)
        thread.start()
        try:
            c = YdszClient(
                f"http://127.0.0.1:{srv.server_address[1]}",
                token="t",
                max_retries=2,
                retry_base_delay=0.001,
            )
            resp = c.search_logs(item=1, date="20261106")
            self.assertTrue(resp["ok"])
            self.assertEqual(calls["n"], 2)
        finally:
            srv.shutdown()

    def test_4xx_not_retried(self):
        calls = {"n": 0}

        class BadHandler(_Handler):
            def do_POST(self):
                calls["n"] += 1
                body = b""
                self.send_response(400)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        srv = HTTPServer(("127.0.0.1", 0), BadHandler)
        thread = threading.Thread(target=srv.serve_forever, daemon=True)
        thread.start()
        try:
            c = YdszClient(
                f"http://127.0.0.1:{srv.server_address[1]}",
                token="t",
                max_retries=3,
                retry_base_delay=0.001,
            )
            with self.assertRaises(APIError):
                c.search_logs(item=1, date="20261106")
            self.assertEqual(calls["n"], 1)
        finally:
            srv.shutdown()


if __name__ == "__main__":
    unittest.main()
