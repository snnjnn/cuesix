#!/usr/bin/env python3
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    server_version = "buffering-test-upstream/1.0"

    def log_message(self, fmt, *args):
        return

    def do_GET(self):
        if self.path.endswith("/large"):
            payload = b"A" * (8 * 1024 * 1024)
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return

        if self.path.endswith("/sse"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.end_headers()
            for i in range(3):
                msg = f"data: tick-{i}\n\n".encode("utf-8")
                self.wfile.write(msg)
                self.wfile.flush()
                time.sleep(0.25)
            return

        if self.path.endswith("/ping"):
            body = b"pong\n"
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_response(404)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_POST(self):
        if self.path.endswith("/upload"):
            content_length = int(self.headers.get("Content-Length", "0"))
            _ = self.rfile.read(content_length)
            body = f"uploaded {content_length}\n".encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        self.send_response(404)
        self.send_header("Content-Length", "0")
        self.end_headers()


def main():
    server = ThreadingHTTPServer(("0.0.0.0", 18080), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
