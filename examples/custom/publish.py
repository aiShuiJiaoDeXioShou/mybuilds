#!/usr/bin/env python3
"""可信仓库的最小一次请求示例；接收端须返回精确发布结果协议。"""
import json
import os
import sys
import urllib.parse
import urllib.request


def main():
    mode = sys.argv[1]
    if mode not in ("upload", "query"):
        raise ValueError("mode")
    with open(os.environ["MYBUILDS_PUBLISH_INPUT"], "rb") as source:
        raw = source.read((64 << 10) + 1)
    if len(raw) > 64 << 10:
        raise ValueError("input")
    original = json.loads(raw)
    endpoint = urllib.parse.urlsplit(os.environ["CUSTOM_ENDPOINT"])
    if endpoint.scheme != "https" or not endpoint.hostname or endpoint.username or endpoint.password or endpoint.query or endpoint.fragment:
        raise ValueError("endpoint")
    url = urllib.parse.urlunsplit(endpoint).rstrip("/") + "/" + urllib.parse.quote(original["intent_id"], safe="")
    data = None
    if mode == "upload":
        # 示例有意限16MiB；大型包应由用户自己的受控传输实现处理。
        with open(original["artifact"]["path"], "rb") as package:
            data = package.read((16 << 20) + 1)
        if len(data) != original["artifact"]["size"] or len(data) > 16 << 20:
            raise ValueError("artifact")
    elif "path" in original["artifact"]:
        raise ValueError("query artifact")
    request = urllib.request.Request(url, data=data, method="POST" if mode == "upload" else "GET")
    request.add_header("Content-Type", "application/octet-stream")
    # 仅本次输入元数据，接收端以intent_id记录幂等键；这里不重试POST。
    metadata = {key: value for key, value in original.items() if key != "artifact"}
    metadata["artifact"] = {key: value for key, value in original["artifact"].items() if key != "path"}
    request.add_header("X-Mybuilds-Metadata", json.dumps(metadata, ensure_ascii=True, separators=(",", ":")))
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, request, fp, code, message, headers, newurl):
            return None
    with urllib.request.build_opener(NoRedirect).open(request, timeout=15) as response:
        result = response.read((64 << 10) + 1)
    if len(result) > 64 << 10:
        raise ValueError("result")
    # 不凭HTTP成功猜测发布结果；系统随后核对原ID/版本/摘要和结构化证据。
    json.loads(result)
    target = os.environ["MYBUILDS_PUBLISH_RESULT"]
    descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "wb") as output:
        output.write(result)
        output.flush()
        os.fsync(output.fileno())


if __name__ == "__main__":
    try:
        main()
    except Exception:
        # 不输出输入、响应、URL、秘密或私有路径；无有效结果会保留unknown。
        sys.exit(1)
