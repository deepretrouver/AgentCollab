# Community release loop / 社区发布循环

Started / 启动：2026-10-30

## Agreed scope / 已确认范围

- License: Apache-2.0.
- Audience: trusted small teams using humans and local native Agents.
- Languages: Chinese and English for introduction, setup and core workflows; audit UI localization coverage before claiming bilingual UI support.
- Preserve existing local data. Run acceptance against isolated storage and ports.
- Do not enable login startup items.
- Report tested platforms, known limitations and publication contents to the owner before GitHub upload.
- GitHub publication is authorized after readiness checks. Scheduled PR maintenance is out of scope for this phase.

## Loop / 执行循环

1. Inspect current source and runtime; reproduce the issue.
2. Implement scoped fixes preserving existing work.
3. Run relevant automated tests.
4. Verify the visible user workflow and record evidence.
5. Reopen failed checks; repeat until release blockers are resolved.

## Gates / 发布关卡

- [ ] Clean source inventory, license and dependency attribution.
- [ ] Reproducible installation and continuous integration.
- [ ] Fresh Server setup and first administrator login.
- [ ] Invite second human, re-login, workspace and room permissions.
- [ ] Client onboarding, native Agent discovery, successful visible delivery.
- [ ] Stop, disconnect, reconnect, restart and persisted device identity.
- [ ] Files, knowledge, project steps, task branch, review and merge.
- [ ] Unauthorized access and cross-owner invocation checks.
- [ ] Chinese and English public documentation and key UI language coverage.
- [ ] Real demo screenshots and reproducible demo scenario.
- [ ] Platform-specific package acceptance and published compatibility table.
- [ ] Public snapshot privacy/secret audit, including embedded assets.
- [ ] Pre-upload owner report.
- [ ] GitHub upload, clean clone verification and release-page check.

## Evidence policy / 证据标准

### Checkpoint / 阶段检查：2026-10-31

The boxes above are final release gates, not a claim that no work has been done.
Keep compound gates open until every part is verified against the final public snapshot.
上方复选框表示最终发布关卡；复合关卡在最终公开快照完成全部验证前不勾选。

| Area / 范围 | Verified / 已验证 | Still required / 仍需完成 |
| --- | --- | --- |
| Source / 源码 | Candidate scanner: 355 paths, no findings; 8 hash-bound reviewed images / 候选扫描无发现 | Final export, dependency notices and publication contents review / 最终导出及发布内容复核 |
| Web / 网页 | 24 test files, 181 tests passed / 24 文件、181 测试通过 | Final exported-source build / 最终导出源码构建 |
| Native collaboration / 原生协作 | OpenCode + Hermes, two task-linked replies and context-aware follow-up / 双 Agent 同话题回复及上下文追问 | Cross-machine and cross-owner real execution; other adapters / 跨机跨主人及其他适配器实测 |
| Mac ARM packages / 安装包 | Actual Server first setup/restart; actual Client invite/login/discovery/reopen with stable device identity / 真包初始化、接入、重开身份保持 | Final reproducible packaging; download/quarantine path; no notarization yet / 正式可复现封装及下载隔离验证，尚未公证 |
| Introduction / 介绍 | Chinese/English guides and reproducible smoke scenarios / 中英指南和验收脚本 | Complete public-safe walkthrough; some deep UI remains Chinese / 完整脱敏演示，部分深层界面仍为中文 |
| Publication / 发布 | Nothing uploaded / 尚未上传 | Owner readiness report before upload; clean clone and release check / 上传前汇报、干净克隆及发布页检查 |

Detailed evidence and limitations: [open-source-readiness.md](open-source-readiness.md).
Windows and Intel Mac binaries must not be presented as accepted merely because package
generation or static installer tests pass. Do not include stale compatibility packages.
Windows、Intel Mac 不能仅因打包或静态测试通过就标为已验收；不混入旧兼容安装包。

Record commands, exit codes, test counts, build identity and visible workflow outcomes. A test passing does not imply an installation package or unsupported operating system has been validated. Do not claim that all bugs have been eliminated. Mark unavailable platforms as unverified and omit their binaries from the initial release.

## Baseline / 初始基线

2026-10-30: Go tests passed; web 119 tests passed; desktop Node tests 19 passed; web production build passed. These are baseline automated checks, not final release acceptance. Existing development directory has substantial uncommitted work and must not be blindly pushed with its local history.
