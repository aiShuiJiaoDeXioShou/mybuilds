# 本机 custom 发布接收端

此接收端配合 [现有发布脚本](../../custom/publish.py) 验证真实二进制上传和只读查询。仅监听 `127.0.0.1` 的 HTTPS；需要 Python 3 和 OpenSSL，不访问应用商店。结果 `uploaded` 表示原包已在本接收端落盘，不代表商店发布或审核通过。

## 启动受信 HTTPS

从 mybuilds 项目根目录运行。材料只写入本次私有临时目录，不修改系统信任库：

```bash
umask 077
export CUSTOM_CASE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mybuilds-custom.XXXXXX")"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -keyout "$CUSTOM_CASE_DIR/ca.key" -out "$CUSTOM_CASE_DIR/ca.pem" \
  -subj '/CN=Mybuilds Owned Custom CA' \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -addext 'keyUsage=critical,keyCertSign,cRLSign'
openssl req -new -newkey rsa:2048 -nodes \
  -keyout "$CUSTOM_CASE_DIR/server.key" -out "$CUSTOM_CASE_DIR/server.csr" \
  -subj '/CN=localhost'
cat > "$CUSTOM_CASE_DIR/server.ext" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:localhost,IP:127.0.0.1
EOF
openssl x509 -req -days 2 -in "$CUSTOM_CASE_DIR/server.csr" \
  -CA "$CUSTOM_CASE_DIR/ca.pem" -CAkey "$CUSTOM_CASE_DIR/ca.key" \
  -CAcreateserial -out "$CUSTOM_CASE_DIR/server.pem" \
  -extfile "$CUSTOM_CASE_DIR/server.ext"
python3 examples/mvp/custom/receiver.py --port 9443 \
  --cert "$CUSTOM_CASE_DIR/server.pem" --key "$CUSTOM_CASE_DIR/server.key" \
  --state "$CUSTOM_CASE_DIR/received"
```

看到 `custom_receiver_ready` 后保持该终端运行；另一个终端通过原发布脚本发请求。接收端不会输出请求头、参数、包内容或私有路径。Ctrl-C 结束接收端，保留 `received` 的证据。

## 接入原发布脚本

沿用 [custom 三步骤配置及说明](../../custom/README.md)，将 `publish.py` 放入实际构建仓库的 `examples/custom/` 并提交。集中案例使用单 `default` 构建，原 `run` 生成 `dist/package.bin`，`artifact` 收集，最后原 `upload target: custom`；节点为已注册的 `flutter-android`，标签 `android-sdk`。保持原脚本与结果协议，不另写客户端。

只在该 upload 的 `env` 中补入私有 CA；Node 的私有 secrets 文件明确提供地址及绝对路径：

```yaml
# upload 步骤的 env
CUSTOM_ENDPOINT: ${CUSTOM_ENDPOINT}
SSL_CERT_FILE: ${CUSTOM_CA_FILE}
```

```dotenv
# Node secrets 文件，权限0600；把路径换为本次实际 ca.pem。
CUSTOM_ENDPOINT=https://127.0.0.1:9443
CUSTOM_CA_FILE=/absolute/private/mybuilds-custom.CASE/ca.pem
```

Python 标准库默认 TLS 校验实际读取 `SSL_CERT_FILE`。接收端和执行该步骤的 Node 须在同一台机器，远程节点的 `127.0.0.1` 指向该节点自身。本例没有关闭证书校验或安装 CA 到系统。若只直接运行原脚本，也须显式提供 `SSL_CERT_FILE`、`CUSTOM_ENDPOINT` 和系统协议规定的 `MYBUILDS_PUBLISH_INPUT` / `MYBUILDS_PUBLISH_RESULT`；后两项在正常运行中由系统生成，不由用户伪造授权。

管理员用自己的应用归属依据绑定项目。以下文件由管理员在核对应用、节点与自有服务后生成；它只记录人工声明，不声称系统已验证外部渠道：

```bash
# 在第二个终端把 CUSTOM_CASE_DIR 设为第一终端创建的实际目录。
umask 077
printf '%s\n' '管理员确认 org.example.custom 归属于本案例自有接收端；节点 flutter-android。' \
  > "$CUSTOM_CASE_DIR/ownership.txt"
python3 - <<'PY'
import hashlib, json, os
from pathlib import Path
case = Path(os.environ['CUSTOM_CASE_DIR'])
value = {'source': 'manual_attested', 'evidence_code': 'ownership_attested',
         'note': '管理员确认本机自有 custom 接收端与应用归属',
         'evidence_sha256': hashlib.sha256((case / 'ownership.txt').read_bytes()).hexdigest()}
with (case / 'verification.json').open('x') as output:
    json.dump(value, output, ensure_ascii=False)
os.chmod(case / 'verification.json', 0o600)
PY
mybuilds project app bind custom-demo --store custom \
  --app-id org.example.custom --node flutter-android \
  --verification-file "$CUSTOM_CASE_DIR/verification.json" --json
mybuilds trigger custom-demo --allow-upload --idempotency-key custom-example-001 --json
```

前提是 `custom-demo` 已按原案例登记可信仓库，Node 已注册且配置了 secrets 文件，管理员凭据从受限 client 配置或环境变量读取。重复同 request key 是同一触发请求；换 key 不会解除未知发布保护。绑定依据为 `manual_attested`，不是外部服务真实性认证。具体上传/查询输入与返回见 [custom 协议](../../../specs/012-custom-workflows/contracts/custom-publish.md)。

## 接收与失败证据

POST 路径为 `/<intent_id>`，正文是原二进制。`X-Mybuilds-Metadata` 限32KiB，必须包含完整 Ref、原授权摘要、应用/版本与产物 ID/大小/SHA；不接收私有 `artifact.path`。包限16MiB，单连接读写不活跃超时15秒；正常调用的总预算仍由原 Run/process 约束。无分块上传、重定向或隐式重试。

接收端以原 Intent 排他创建0700目录，校验正文 SHA，写入0600的 `package.bin` 与 `receipt.json` 并 fsync 文件和目录后返回 schema1的精确关联回执。同 Intent 的完整重复请求只有元数据和包一致时才返回原回执；不会覆盖未知记录。GET 只校验并读取该原记录和包，不创建目录、不重传、不写查询记录。缺记录返回404 `record_unknown`，不能据此证明原发布未发生或清除系统 guard。

不合法关联返回409；保存不确定返回503，部分文件保留。未知结果应沿原 Intent 查询并由管理员根据真实证据处理，不能重新触发代替核对。本机回环服务没有额外认证，仅供自有案例；不要作为公共上传服务暴露。外部渠道仍由用户在集中人工验收时验证。
