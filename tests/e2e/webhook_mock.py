#!/usr/bin/env python3
"""Deterministic webhook receiver for externally reachable E2E runs."""

import argparse
import base64
import hashlib
import hmac
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class State:
    def __init__(self):
        self.lock = threading.Lock()
        self.secret = b""
        self.fail_first = 0
        self.attempts = []
        self.deliveries = []

    def reset(self):
        with self.lock:
            self.attempts.clear()
            self.deliveries.clear()


STATE = State()


class Handler(BaseHTTPRequestHandler):
    server_version = "pulse-webhook-mock/1"

    def log_message(self, fmt, *args):
        return

    def send_json(self, status, value):
        encoded = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def read_json(self):
        length = int(self.headers.get("Content-Length", "0"))
        return json.loads(self.rfile.read(length))

    def do_GET(self):
        if self.path == "/health":
            self.send_json(200, {"status": "ok"})
            return
        if self.path == "/deliveries":
            with STATE.lock:
                self.send_json(200, {"attempts": list(STATE.attempts), "deliveries": list(STATE.deliveries)})
            return
        self.send_json(404, {"error": "not found"})

    def do_POST(self):
        if self.path == "/reset":
            STATE.reset()
            self.send_json(200, {"status": "reset"})
            return
        if self.path == "/config":
            try:
                config = self.read_json()
                secret = base64.urlsafe_b64decode(config.get("secretB64", "") + "===")
                fail_first = int(config.get("failFirst", 0))
                if fail_first < 0:
                    raise ValueError("failFirst must be non-negative")
            except (ValueError, TypeError, json.JSONDecodeError) as exc:
                self.send_json(400, {"error": str(exc)})
                return
            with STATE.lock:
                STATE.secret = secret
                STATE.fail_first = fail_first
            self.send_json(200, {"status": "configured"})
            return

        try:
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length)
            event = json.loads(body)
            event_id = event["eventId"]
        except (ValueError, KeyError, json.JSONDecodeError) as exc:
            self.send_json(400, {"error": str(exc)})
            return

        timestamp = self.headers.get("X-Pulse-Timestamp", "")
        signature = self.headers.get("X-Pulse-Signature", "")
        with STATE.lock:
            attempt_number = len(STATE.attempts) + 1
            STATE.attempts.append({"eventId": event_id, "number": attempt_number})
            expected = hmac.new(STATE.secret, (timestamp + ".").encode() + body, hashlib.sha256).hexdigest()
            valid = bool(STATE.secret) and hmac.compare_digest(expected, signature)
            should_fail = attempt_number <= STATE.fail_first
            if not should_fail:
                STATE.deliveries.append({"eventId": event_id, "signatureValid": valid, "body": event})
        if should_fail:
            self.send_json(500, {"error": "deterministic failure", "attempt": attempt_number})
        else:
            self.send_json(204, {})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=8090)
    args = parser.parse_args()
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"webhook mock listening on {args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
