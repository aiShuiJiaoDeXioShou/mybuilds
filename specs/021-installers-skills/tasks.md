# 任务：跨平台安装与操作技能

## Phase 1: Setup
- [x] T001 核对原则、部署架构及平台边界，记录 specs/021-installers-skills/research.md。

## Phase 2: Foundation
- [x] T002 实现 scripts/release.py 生成预编译包与 SHA256SUMS（FR-004、FR-008）。

## Phase 3: US1 客户端
- [x] T003 [US1] 实现 scripts/install-client.sh、scripts/install-client.ps1 下载校验及 scripts/install.py 客户端安装与PATH配置（FR-001、FR-004、FR-006）。
- [x] T004 [US1] 在 scripts/test-install.py 验证重复安装、私有配置、路径和失败保护（SC-001、FR-004）。

## Phase 4: US2 服务端
- [x] T005 [US2] 实现 scripts/install-server.sh、scripts/install-server.ps1、scripts/install.sh 和 scripts/install.py 服务初始化与平台服务管理（FR-002、FR-003、FR-005）。
- [x] T006 [US2] 通过 scripts/test-install.py 验证初始化与重跑、真实控制端/Agent启动（SC-002）。

## Phase 5: US3 技能与发布
- [x] T007 [US3] 编写并验证 skills/mybuilds-deploy/SKILL.md 与 skills/mybuilds-operate/SKILL.md（FR-007）。
- [x] T008 [US3] 更新 README.md、docs/INSTALL.md、docs/CLI.md，记录安装和Windows边界（FR-001–007）。
- [ ] T009 [US3] 检查后提交推送公开仓库与发行包，在 specs/021-installers-skills/validation.md 记录公开下载与部署证据（FR-008、FR-009、SC-003、SC-004）。

## 依赖与实施
T001 → T002 → T003/T005 → T004/T006 → T007/T008 → T009。主代理串行改共享安装器；独立平台构建可并行执行工具，不并发改文件。先离线包安装验收，再发布和真实部署；验证记录最后补充，不能把交叉编译写成真实平台运行。
