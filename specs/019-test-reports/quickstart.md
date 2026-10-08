# 019 验收指南

本指南描述019整功能验收门。当前从008已验收基线 `504dc6fa8581f74a15ecc146a474976d5ae33a22`实施，本地8个实际CLI场景、中央双库各92项应用检查点、最终20故障192断言及macOS/Linux实机门已通过，完整原XML/秘密/预算/停止确认与全量检查见[验证记录](validation.md)。各输入负例由对应真实文件/进程/HTTP/双库行为测试覆盖，不将所有负例虚称独立CLI场景。不借005未验收资源，不访问未知宿主签名身份、不修改共享数据库/系统信任。

## 构建与私有夹具

在项目根目录准备自有0700临时目录；构建实际客户端/控制端/Agent，不以mockexecutor替代：

```bash
project_dir="$(pwd)"
umask 077
FIXTURE="$(mktemp -d)"
chmod 0700 "$FIXTURE"
mkdir -m 0700 "$FIXTURE/work"
go build -o "$FIXTURE/mybuilds" ./cmd/mybuilds
go build -o "$FIXTURE/mybuilds-server" ./cmd/mybuilds-server
go build -o "$FIXTURE/mybuilds-agent" ./cmd/mybuilds-agent
```

## 本地替换、失败哨兵与post不可改证据

以下相同路径先生成1个通过case，再改为5个case（2通过、1失败、1错误、1跳过）。第二次检查应替换而非合计6个。前置准备无文件不失败；测试生成失败后普通哨兵不执行，选failure/always；post将工作树改为通过不能改变原seal或XML快照：

```bash
cat > "$FIXTURE/work/mybuilds.yml" <<'YAML'
version: 1
builds:
  junit:
    reports:
      junit:
        paths: [results/*.xml]
        required: true
    timeout: 30s
    steps:
      - kind: run
        name: prepare
        run: mkdir -p results
      - kind: run
        name: first-report
        run: |
          printf '<testsuite tests="1" failures="0"><testcase name="initial" time="0.1"/></testsuite>' > results/result.xml
      - kind: run
        name: replace-report
        run: |
          cat > results/result.xml <<'XML'
          <testsuites tests="5" failures="1" errors="1" skipped="1">
            <testsuite tests="2" failures="0"><testcase name="pass-a" time="0.1"/><testcase name="pass-b" time="0.2"/></testsuite>
            <testsuite tests="3" failures="1" errors="1" skipped="1">
              <testcase name="failed" time="0.3"><failure message="expected mismatch">bounded diagnostic</failure></testcase>
              <testcase name="errored" time="0.4"><error message="fixture error"/></testcase>
              <testcase name="skipped" time="0"><skipped/></testcase>
            </testsuite>
          </testsuites>
          XML
      - kind: run
        name: must-not-run
        run: printf unexpected > sentinel.txt
    post:
      success:
        - kind: run
          name: success-must-not-run
          run: printf unexpected > success.txt
      failure:
        - kind: run
          name: failure-observed
          run: printf failure_marker
      always:
        - kind: run
          name: post-diagnostic-rewrite
          run: |
            printf '<testsuite tests="1"><testcase name="post-pass"/></testsuite>' > results/result.xml
            printf cleanup_marker
YAML

cd "$FIXTURE/work"
if "$FIXTURE/mybuilds" run --file ./mybuilds.yml --build junit > "$FIXTURE/local.json" 2> "$FIXTURE/local.log"; then
  printf '错误：失败报告不应返回成功\n' >&2
  exit 1
fi
test ! -e sentinel.txt
test ! -e success.txt
cat "$FIXTURE/local.json"
cat "$FIXTURE/local.log"
cd "$project_dir"
```

预期build failed/report_failed；Counts tests=5/failures=1/errors=1/skipped=1/duration_ns=1000000000，真实ordinary run仍有原ExitCode0/Started/StopConfirmed。reports已seal，原XML摘要对应失败5-case快照，工作树结果是post-pass但不放行。Run.ResultDir私有reports manifest中的当前文件与公开安全ID/Size/SHA一致，不直接下载工作树源。post日志failure_marker/cleanup_marker存在，success不执行。

把replace-report最后加 `exit 7` 后用新夹具再跑：原命令failed/exit/ExitCode7保留，仍尝试收新报告、summary正确；post错误不覆盖原Reason。另用只通过/合法0case报告和required=false缺失验证成功，未配置reports回归007既有行为。

## 新鲜性及输入负例

每项使用新的自有夹具/实际脚本，不删除用户旧文件绕门：

| 真实场景 | 必须结果 |
|---|---|
| 开始前预置通过XML，选中步骤不写报告 | 旧identity/mtime/SHA不变，不计入；required true最终report_missing |
| 同内容新inode重建、改写保留mtime、同路径重复检查 | 本次身份或摘要证据识别；按路径最新版一次计数 |
| accepted文件被删、多个glob有缺失 | final移除；true按每模式缺失失败，false仅豁免缺失 |
| --step只跑准备，已有旧通过XML | 不借旧报告或跳过预检查，最终required失败 |
| required=false且有坏XML、矛盾计数、重复case结局 | failed/report_invalid，不当optional通过 |
| symlink目录/leaf、FIFO、替换竞争、../绝对/盘符/控制字符 | 固定安全失败、无越界读取，不阻塞，不删除未知文件 |
| DOCTYPE/实体/未知编码/深度65/8MiB+1/文件65/100001cases | 有界失败、不联网、无rawXML错误 |
| 声明secret出现在rawXML、实体编码属性/文本或相对路径 | report_secret，原XML不公开/不上传，不“脱敏重写原XML” |
| cancel/timeout/cleanup失败后报告检查错误 | 原原因不被覆盖，失权/保存失败不执行下一步或always |
| precheck零动作失败、build/steps全when false、无reports | 无假seal/required失败/报告扫描，保持原结果 |

用已声明环境变量（如APP_REPORT_SECRET）注入自有随机secret，扫描stdout/stderr/result JSON/Node事件/用户详情/下载；不得从宿主猜秘密。失败诊断须有限、脱敏、UTF-8安全，XML原字节/Size/SHA保持；不把输出里有报告字样当验收。

## 真实远端与中央下载

复用[007私有Server/Client/Agent指南](../007-node-agents/quickstart.md)：独立token/session/data_dir、实际verified HTTPS/私有CA、受信固定Git、无runner项目显式default_node。以下假定该指南的client_bin、FIXTURE/client.yml及自己的节点demo-generic已准备；为报告夹具另设REPORT_WORK指向上述work，不复用他人目录。

```bash
REPORT_WORK="$FIXTURE/work"
git -C "$REPORT_WORK" init --initial-branch=main
git -C "$REPORT_WORK" add mybuilds.yml
git -C "$REPORT_WORK" -c user.name=ReportFixture -c user.email=report@example.invalid commit -m '本次JUnit报告'
report_sha="$(git -C "$REPORT_WORK" rev-parse HEAD)"
"$client_bin" --config "$FIXTURE/client.yml" project init report-demo \
  --repo "$REPORT_WORK" --nodes demo-generic --default-node demo-generic
"$client_bin" --config "$FIXTURE/client.yml" trigger report-demo --build junit \
  --ref "$report_sha" --idempotency-key report-actual-1 --json > "$FIXTURE/report-batch.json"
BUILD_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["builds"][0]["id"])' "$FIXTURE/report-batch.json")"
"$client_bin" --config "$FIXTURE/client.yml" logs "$BUILD_ID" --follow --stream-timeout 15m
"$client_bin" --config "$FIXTURE/client.yml" build show "$BUILD_ID" --json
"$client_bin" --config "$FIXTURE/client.yml" artifact ls "$BUILD_ID" --json > "$FIXTURE/report-files.json"
ARTIFACT_ID="$(python3 -c 'import json,sys; print(next(x["id"] for x in json.load(open(sys.argv[1]))["items"] if x.get("purpose")=="junit"))' "$FIXTURE/report-files.json")"
mkdir -m 0700 "$FIXTURE/report-downloads"
"$client_bin" --config "$FIXTURE/client.yml" artifact download "$ARTIFACT_ID" --output "$FIXTURE/report-downloads/result.xml"
```

跨主机使用双方均可读取的受信SSH/匿名HTTPS仓库代替本地路径，控制端与节点分别提供显式SSH材料（控制端GIT_SSH_KEY_FILE/GIT_SSH_KNOWN_HOSTS_FILE，Agent MYBUILDS_GIT_SSH_KEY/MYBUILDS_GIT_KNOWN_HOSTS）。实际macOS和Linux各跑通过及上述失败链，核对两个详情Counts/Outcome和完整原XML。停自己的Agent后中央报告仍可下载，下载是失败5-case原内容而非post-pass；文件大小/SHA与seal对应，覆盖已有目标拒绝。

admin/approver可读，trigger/user-node交叉身份拒绝；Node只能同fence上传与确认，不可读用户报告。真实篡改stage SHA、短流、report summary/ID冲突、旧revision/旧lease、假SourceIndex/未Started、只summary/缺原XML、中央坏文件均不能成为passed/terminal；SQL失败孤立文件不可见，重传同ID只认完整确认。丢checked/seal/PUT响应按原receipt恢复，无重复文件/汇总。

## 双库、预算与008兼容

自有SQLite与独立PostgreSQL数据库运行同套Store/API/三二进制，用实际会话锁/old expires/20幂等触发/重启/取消/失权/持久化失败验证。普通制品128份、JUnit按max_files单独计数，合计4GiB和XML累计64MiB不绕过；10s纯检查上限与ordinary剩余ns较小者生效，上传/receipt仍计ordinary，不转post预算。剩余0拒启动，最后确认耗尽预算为timeout。

最终terminal完整精确Reports seal digest/IDs/Counts以及原日志/ArtifactSteps/cursors核对后才能产生008 StopKnown receipt；尚未确认文件或checked不能清journal。retry沿原SHA/Reports配置但新执行不沿旧pass，恢复不重读post改写XML。008已在504dc6验收集成；019沿其真实终态回执与原快照重试入口核对报告字段，未造占位恢复逻辑。

```bash
go test ./...
go vet ./...
go test -race ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$FIXTURE/client-linux" ./cmd/mybuilds
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$FIXTURE/client-windows.exe" ./cmd/mybuilds
```

记录实际Go/两库/节点版本、固定SHA、UTC命令/退出码、实际报告字节/摘要与安全断言；无报告回归、全部18FR/6SC/12AC映射、README/历史/validation与converge通过后整019一次本地提交，无push。报告通过/封存不等于商店或Apple签名验收，不启动未交付发布能力。

## 2026-10-08 大量报告

旧配置自动提升为256份；大型工程在相应build配置：

```yaml
reports:
  junit:
    paths: [build/test-results/**/*.xml]
    required: true
    max_files: 1024
```

数量为1–1024整数；默认256份/第257份失败，自定义1024份/第1025份失败。普通制品仍128份，报告另外计数，XML8MiB/总64MiB/10万cases等其他预算保持。消息8MiB、journal64MiB/100万节点独立有界，不代表任意诊断与审批历史组合无限可用。
