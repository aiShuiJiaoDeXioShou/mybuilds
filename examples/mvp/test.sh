#!/usr/bin/env bash
set -uo pipefail
export CI=true FLUTTER_SUPPRESS_ANALYTICS=true
mkdir -p results
rm -f results/flutter.xml
flutter --no-version-check --suppress-analytics pub get
status=$?
if [[ "$status" == 0 ]]; then
  flutter --no-version-check --suppress-analytics test --no-pub test/smoke_test.dart \
    --dart-define "MYBUILDS_CHANNEL=$APP_CHANNEL"
  status=$?
fi
# 记录实际Flutter命令的聚合结果；不伪造框架逐case计数或诊断。
if [[ "$status" == 0 ]]; then
  printf '%s\n' '<testsuite name="flutter-command"><testcase name="flutter-test-command"/></testsuite>' > results/flutter.xml
else
  printf '%s\n' '<testsuite name="flutter-command"><testcase name="flutter-test-command"><failure message="Flutter命令失败；见原步骤日志"/></testcase></testsuite>' > results/flutter.xml
fi
exit "$status"
