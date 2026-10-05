#!/usr/bin/env python3
"""从实际CLI模板创建独立Flutter双平台验收仓库，不启动服务或发布。"""
import pathlib
import shutil
import subprocess
import sys


def main():
    if len(sys.argv) != 3:
        raise SystemExit("用法：python3 prepare.py MYBUILDS绝对路径 新案例目录")
    binary = pathlib.Path(sys.argv[1]).resolve(strict=True)
    target = pathlib.Path(sys.argv[2]).absolute()
    if target.exists():
        raise SystemExit("案例目录已存在，拒绝覆盖")
    source = pathlib.Path(__file__).resolve().parents[2]
    target.mkdir(mode=0o700, parents=True)
    repo = target / "repo"
    shutil.copytree(source / "examples/flutter", repo)
    subprocess.run([str(binary), "init", "--framework", "flutter", "--platform", "android,ios"], cwd=repo, check=True)
    pipeline = (repo / "mybuilds.yml").read_text()
    test_step = """    reports:
      junit: {paths: [results/flutter.xml], required: true}
    post:
      timeout: 2m
      always:
        - kind: run
          name: case-post
          run: printf '本次案例收尾完成\\n'
    steps:
      - kind: run
        name: flutter-tests
        shell: bash
        run: ./ci/test.sh
"""
    pipeline = pipeline.replace("    steps:\n", test_step)
    for name in ("android", "ios"):
        pipeline = pipeline.replace(
            f"  {name}:\n",
            f"  {name}:\n    when:\n      changes: [lib/**, pubspec.*, {name}/**, ci/**, test/**, mybuilds.yml]\n",
        )
    # 模板自身参数与命令保留，只追加集中人工验收所需步骤。
    android, ios = pipeline.split("  ios:\n", 1)
    approval = """      - kind: approval
        name: release-approval
        notify: false
        when: {params: {channel: store}}
"""
    android += approval + """      - kind: upload
        name: google-play
        when: {params: {channel: store}}
        target: google_play
        file: build/mybuilds-flutter/app-release.aab
        app_identifier: "{{application_id}}"
        track: internal
        credentials: "${GOOGLE_PLAY_JSON}"
"""
    ios += approval + """      - kind: upload
        name: app-store
        when: {params: {channel: store}}
        target: app_store
        file: "{{ios.output_dir}}/App.ipa"
        app_identifier: "{{bundle_id}}"
        credentials: "${APP_STORE_KEY}"
        submit_for_review: false
        automatic_release: false
"""
    (repo / "mybuilds.yml").write_text(android + "  ios:\n" + ios)
    (repo / "ci").mkdir()
    shutil.copyfile(source / "examples/mvp/test.sh", repo / "ci/test.sh")
    (repo / "ci/test.sh").chmod(0o755)
    (repo / "test").mkdir()
    shutil.copyfile(source / "examples/mvp/smoke_test.dart", repo / "test/smoke_test.dart")
    for name in ("server.yml", "client.yml", "agent-android.yml", "agent-ios.yml", "settings.yml", "profile-settings.yml"):
        shutil.copyfile(source / "examples/mvp" / name, target / name)
        (target / name).chmod(0o600)
    print("案例已生成；先编辑应用标识与节点材料，再按acceptance.md登记并运行。")


if __name__ == "__main__":
    main()
