#!/usr/bin/env python3
"""Tiny static file server with SPA fallback, for LOCAL dashboard preview.

Serves --dir; any missing path under /calibration/ falls back to
/calibration/index.html, so clean routes like /calibration/run/<id> work on a
direct load locally -- mirroring what 404.html does on GitHub Pages.

Usage:  devserver.py --dir SITE_ROOT [--port 8000]
"""
import argparse
import http.server
import os


class Handler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if not os.path.exists(self.translate_path(path)) and "/calibration/" in path:
            self.path = "/calibration/index.html"
        return super().do_GET()

    def log_message(self, *a):
        pass


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", required=True)
    ap.add_argument("--port", type=int, default=8000)
    args = ap.parse_args()
    os.chdir(args.dir)
    http.server.HTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
