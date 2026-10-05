#!/usr/bin/env python3
"""对真实仓库HEAD发送generic Webhook，不执行构建或商店命令。"""
import argparse
import hashlib
import hmac
import ipaddress
import json
import os
import re
import signal
import ssl
import stat
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request


def read_file(filename, limit, private=False):
    flags = os.O_RDONLY | os.O_NONBLOCK | getattr(os, "O_NOFOLLOW", 0)
    fd = os.open(filename, flags)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size > limit:
            raise ValueError("材料必须为有限普通文件")
        if private and (before.st_uid != os.getuid() or stat.S_IMODE(before.st_mode) != 0o600 or before.st_nlink != 1):
            raise ValueError("秘密响应文件必须由当前用户独占且权限为0600")
        content = bytearray()
        while len(content) <= limit:
            part = os.read(fd, min(65536, limit + 1 - len(content)))
            if not part:
                break
            content.extend(part)
        after = os.fstat(fd)
        path = os.stat(filename, follow_symlinks=False)
        if len(content) > limit or len(content) != before.st_size or (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns) or (after.st_dev, after.st_ino) != (path.st_dev, path.st_ino):
            raise ValueError("材料读取期间身份或内容改变")
        return bytes(content)
    finally:
        os.close(fd)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("JSON字段重复")
        result[key] = value
    return result


def deadline(_signum, _frame):
    raise TimeoutError("Webhook HTTP超过30秒")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, _request, _fp, _code, _message, _headers, _url):
        return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default="repo", help="已经提交的真实Git仓库")
    parser.add_argument("--secret-file", default="hook-rotation.private.json", help="rotate --json的私有响应文件")
    parser.add_argument("--url", default="http://127.0.0.1:8787/hook/flutter-demo")
    parser.add_argument("--ca-file", help="HTTPS额外CA PEM文件")
    parser.add_argument("--delivery", default=os.environ.get("MYBUILDS_HOOK_DELIVERY"), help="显式投递ID；亦可从MYBUILDS_HOOK_DELIVERY读取")
    args = parser.parse_args()
    if not args.delivery or not re.fullmatch(r"[A-Za-z0-9._:-]{1,128}", args.delivery):
        raise ValueError("需要1至128字节的显式投递ID")
    url = urllib.parse.urlsplit(args.url)
    if url.username or url.password or url.query or url.fragment or not url.hostname or url.path != "/hook/flutter-demo" or url.scheme not in ("http", "https"):
        raise ValueError("需要无凭据或query的flutter-demo Hook URL")
    _ = url.port
    if url.scheme == "http":
        try:
            loopback = ipaddress.ip_address(url.hostname).is_loopback
        except ValueError:
            loopback = url.hostname == "localhost"
        if not loopback or args.ca_file:
            raise ValueError("HTTP只允许loopback；额外CA仅用于HTTPS")
    if not hasattr(signal, "setitimer"):
        raise ValueError("本脚本需要macOS或Linux的有界HTTP计时器")
    material = json.loads(read_file(args.secret_file, 65536, private=True), object_pairs_hook=unique_object)
    secret = material.get("secret") if isinstance(material, dict) else None
    if not isinstance(secret, str) or not 32 <= len(secret.encode()) <= 4096 or any(ord(ch) < 32 or ord(ch) == 127 for ch in secret):
        raise ValueError("需要rotate响应顶层合法secret")
    env = {key: os.environ[key] for key in ("PATH", "LANG", "LC_ALL") if key in os.environ}
    env.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_SYSTEM=os.devnull, GIT_TERMINAL_PROMPT="0")
    result = subprocess.run(["git", "-c", "core.hooksPath=" + os.devnull, "-C", args.repo, "rev-parse", "--verify", "HEAD^{commit}"], env=env, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=10, check=True)
    sha = result.stdout.decode("ascii").strip()
    if not re.fullmatch(r"(?:[0-9a-f]{40}|[0-9a-f]{64})", sha):
        raise ValueError("仓库HEAD不是完整提交SHA")
    body = json.dumps({"repository": "flutter-demo", "ref": "refs/heads/main", "before": "0" * len(sha), "after": sha}, separators=(",", ":")).encode()
    headers = {"Content-Type": "application/json", "X-Mybuilds-Event": "push", "X-Mybuilds-Delivery": args.delivery,
               "X-Mybuilds-Signature": "sha256=" + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()}
    context = ssl.create_default_context()
    if args.ca_file:
        context.load_verify_locations(cadata=read_file(args.ca_file, 1 << 20).decode("ascii"))
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(), urllib.request.HTTPSHandler(context=context))
    request = urllib.request.Request(args.url, body, headers)
    old_handler = signal.signal(signal.SIGALRM, deadline)
    signal.setitimer(signal.ITIMER_REAL, 30)
    try:
        with opener.open(request, timeout=30) as response:
            raw = response.read(65537)
            if len(raw) > 65536:
                raise ValueError("Webhook响应超过64KiB")
            receipt = json.loads(raw, object_pairs_hook=unique_object)
            if not isinstance(receipt, dict):
                raise ValueError("Webhook响应必须为安全对象")
            safe = {key: value for key, value in receipt.items() if key in ("event_id", "window_id", "status", "reason", "replayed")}
            if any(not isinstance(value, (str, bool)) or isinstance(value, str) and len(value) > 256 for value in safe.values()):
                raise ValueError("Webhook响应字段不合法")
            print(json.dumps({"http_status": response.status, "sha": sha, "receipt": safe}, ensure_ascii=False))
    except urllib.error.HTTPError as error:
        raise ValueError("Webhook发送被拒绝（HTTP " + str(error.code) + "）") from None
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, old_handler)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, TimeoutError, RecursionError, subprocess.SubprocessError):
        print("Webhook发送失败：请检查私有响应、仓库HEAD、投递ID、URL或CA；未输出秘密。", file=sys.stderr)
        raise SystemExit(1)
