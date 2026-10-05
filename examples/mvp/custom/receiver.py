#!/usr/bin/env python3
"""自有回环 HTTPS 接收端；用于真实 custom 脚本案例，不访问商店。"""
import argparse
import hashlib
import http.server
import json
import os
import re
import ssl
import stat
import uuid

LIMIT = 16 << 20
HEX = re.compile(r"[0-9a-f]{64}\Z")
NOFOLLOW = os.O_NOFOLLOW | os.O_NONBLOCK


def pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise ValueError("duplicate")
        result[key] = value
    return result


def load(raw):
    value = json.loads(raw, object_pairs_hook=pairs,
                       parse_constant=lambda _: (_ for _ in ()).throw(ValueError("number")))
    def visit(item, depth=0):
        if depth > 12 or item is None:
            raise ValueError("structure")
        if isinstance(item, dict):
            for child in item.values():
                visit(child, depth + 1)
        elif isinstance(item, list):
            for child in item:
                visit(child, depth + 1)
    visit(value)
    return value


def encode(value):
    return json.dumps(value, ensure_ascii=True, sort_keys=True, separators=(",", ":")).encode()


def uid(value):
    return isinstance(value, str) and str(uuid.UUID(value)) == value


def text(value):
    return isinstance(value, str) and 0 < len(value) <= 256 and all(ord(c) >= 32 and ord(c) != 127 for c in value)


def positive(value):
    return type(value) is int and 0 < value <= (1 << 63) - 1


def metadata(raw, intent):
    if len(raw.encode()) > 32 << 10:
        raise ValueError("metadata")
    data = load(raw)
    required = {"schema", "intent_id", "authorization_digest", "ref", "app_identifier",
                "version_name", "version_code", "artifact", "report_ids", "params"}
    if not isinstance(data, dict) or not required <= data.keys() or data.keys() - required - {"channel", "report_seal_digest"}:
        raise ValueError("fields")
    if type(data["schema"]) is not int or data["schema"] != 1 or data["intent_id"] != intent:
        raise ValueError("identity")
    if not HEX.fullmatch(data["authorization_digest"]) or not text(data["app_identifier"]) or not text(data["version_name"]) or not positive(data["version_code"]):
        raise ValueError("identity")
    ref = data["ref"]
    keys = {"node_id", "session_id", "build_id", "attempt_id", "lease_id", "epoch"}
    if not isinstance(ref, dict) or ref.keys() != keys or not positive(ref["epoch"]) or not all(uid(ref[k]) for k in keys - {"epoch"}):
        raise ValueError("ref")
    artifact = data["artifact"]
    if not isinstance(artifact, dict) or artifact.keys() != {"id", "size", "sha256"} or not uid(artifact["id"]) or not positive(artifact["size"]) or artifact["size"] > LIMIT or not HEX.fullmatch(artifact["sha256"]):
        raise ValueError("artifact")
    if not isinstance(data["report_ids"], list) or len(data["report_ids"]) > 128 or not all(uid(v) for v in data["report_ids"]):
        raise ValueError("reports")
    if not isinstance(data["params"], dict) or not all(isinstance(k, str) and isinstance(v, str) for k, v in data["params"].items()):
        raise ValueError("params")
    if "channel" in data and not text(data["channel"]):
        raise ValueError("channel")
    if "report_seal_digest" in data and not HEX.fullmatch(data["report_seal_digest"]):
        raise ValueError("reports")
    return data


def directory(path, parent=None):
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | NOFOLLOW, dir_fd=parent)
    info = os.fstat(fd)
    if info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
        os.close(fd)
        raise ValueError("directory")
    return fd


def read_file(parent, name, maximum):
    fd = os.open(name, os.O_RDONLY | NOFOLLOW, dir_fd=parent)
    with os.fdopen(fd, "rb") as source:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_uid != os.getuid() or stat.S_IMODE(before.st_mode) != 0o600 or before.st_nlink != 1 or before.st_size > maximum:
            raise ValueError("file")
        raw = source.read(maximum + 1)
        after = os.fstat(fd)
        if any(getattr(before, name) != getattr(after, name) for name in ("st_dev", "st_ino", "st_mode", "st_uid", "st_nlink", "st_size", "st_mtime_ns", "st_ctime_ns")) or len(raw) != before.st_size:
            raise ValueError("changed")
        return raw


def write_file(parent, name, raw):
    fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | NOFOLLOW, 0o600, dir_fd=parent)
    with os.fdopen(fd, "wb") as target:
        target.write(raw)
        target.flush()
        os.fsync(fd)


def stored(parent, expected):
    record = load(read_file(parent, "receipt.json", 64 << 10))
    if record["metadata"] != expected:
        raise ValueError("conflict")
    package = read_file(parent, "package.bin", LIMIT)
    if len(package) != expected["artifact"]["size"] or hashlib.sha256(package).hexdigest() != expected["artifact"]["sha256"]:
        raise ValueError("package")
    return record["result"]


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass  # 不记录 URL、参数、头或原包。

    def reply(self, status, value):
        raw = encode(value)
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(raw)
        self.close_connection = True

    def process(self, upload):
        child = None
        try:
            intent = self.path.removeprefix("/")
            if self.path != "/" + intent or not uid(intent) or len(self.headers.get_all("X-Mybuilds-Metadata", [])) != 1 or self.headers.get("Transfer-Encoding"):
                raise ValueError("request")
            data = metadata(self.headers["X-Mybuilds-Metadata"], intent)
            lengths = self.headers.get_all("Content-Length", [])
            if upload:
                if len(lengths) != 1 or not lengths[0].isascii() or not lengths[0].isdigit() or int(lengths[0]) != data["artifact"]["size"]:
                    raise ValueError("length")
                package = self.rfile.read(int(lengths[0]))
                if len(package) != int(lengths[0]) or hashlib.sha256(package).hexdigest() != data["artifact"]["sha256"]:
                    raise ValueError("sha")
                print("custom_post_received", flush=True)
                try:
                    os.mkdir(intent, 0o700, dir_fd=self.server.state)
                except FileExistsError:
                    child = directory(intent, self.server.state)
                    self.reply(200, stored(child, data))
                    return
                child = directory(intent, self.server.state)
                # 排他目录中的部分失败也保留，不覆盖未知记录或重发远端操作。
                result = {"schema": 1, "intent_id": intent,
                          "authorization_digest": data["authorization_digest"],
                          "app_identifier": data["app_identifier"], "artifact_id": data["artifact"]["id"],
                          "artifact_sha256": data["artifact"]["sha256"],
                          "version_name": data["version_name"], "version_code": data["version_code"],
                          "status": "uploaded", "evidence_code": "remote_receipt",
                          "remote_id": intent, "action_confirmed": True}
                write_file(child, "package.bin", package)
                write_file(child, "receipt.json", encode({"metadata": data, "result": result}))
                os.fsync(child)
                os.fsync(self.server.state)
                self.reply(200, result)
            else:
                if lengths and (len(lengths) != 1 or lengths[0] != "0"):
                    raise ValueError("body")
                try:
                    child = directory(intent, self.server.state)
                except FileNotFoundError:
                    self.reply(404, {"error": "record_unknown"})
                    return
                self.reply(200, stored(child, data))
        except (ValueError, TypeError, KeyError, AttributeError):
            self.reply(409, {"error": "request_or_record_invalid"})
        except OSError:
            self.reply(503, {"error": "record_unconfirmed"})
        finally:
            if child is not None:
                os.close(child)

    def do_POST(self):
        self.process(True)

    def do_GET(self):
        self.process(False)


class Server(http.server.HTTPServer):
    def get_request(self):
        connection, address = super().get_request()
        connection.settimeout(15)
        try:
            wrapped = self.tls.wrap_socket(connection, server_side=True)
            return wrapped, address
        except Exception:
            connection.close()
            raise

    def handle_error(self, *_):
        print("custom_connection_unconfirmed", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=9443)
    parser.add_argument("--cert", required=True)
    parser.add_argument("--key", required=True)
    parser.add_argument("--state", required=True)
    args = parser.parse_args()
    if not 1 <= args.port <= 65535:
        raise ValueError("port")
    try:
        os.mkdir(args.state, 0o700)
    except FileExistsError:
        pass
    state = directory(args.state)
    try:
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.minimum_version = ssl.TLSVersion.TLSv1_2
        tls.load_cert_chain(args.cert, args.key)
        with Server(("127.0.0.1", args.port), Handler) as server:
            server.state, server.tls = state, tls
            print("custom_receiver_ready", flush=True)
            server.serve_forever()
    finally:
        os.close(state)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        pass
    except Exception:
        raise SystemExit("custom_receiver_failed")
