# Contributing / 参与贡献

AgentCollab is preparing its first community preview. Include operating system, Server/Client versions, reproduction steps, expected and actual behavior in bug reports. Remove credentials and private team data from logs and screenshots.

AgentCollab 正在准备首个社区预览版。问题反馈请包含操作系统、Server/Client 版本、复现步骤、预期和实际结果。日志与截图请移除凭证及团队私人数据。

## Development / 开发

Use the Go version in `go.mod`, Node.js 22.12+ and a C compiler for the Hub SQLite driver. Git is required for project repositories.

```sh
npm ci --prefix web
npm run test:run --prefix web
make web-build
go test ./...
go vet ./...
node --test desktop/*.test.cjs server-desktop/*.test.cjs
```

Run development against isolated data directories. Add regression coverage for permissions, task delivery, file handling and device identity changes. Verify visible workflows for UI changes.

开发使用独立数据目录。权限、任务交付、文件处理和设备身份变更需要回归测试；界面变更还需验证用户可见流程。

## Safe reports / 安全反馈

Use the bug or feature template when the public repository is available. Never attach a
team database, `.env`, device credentials, browser cookies, full invitation URL or raw home
directory. Replace private names and addresses in screenshots; keep only the relevant log
lines. A private vulnerability-reporting route will be documented once verified—do not
post exploit details or secrets in public Issues.

公开仓库可用后使用问题或建议模板。不要附团队数据库、`.env`、设备凭证、Cookie、
完整邀请链接或整个主目录。截图中的私人姓名和地址请替换，日志只保留相关行。
私密漏洞反馈渠道确认后会另行说明；请勿在公开 Issue 中发布利用细节或秘密。

Dependency updates must keep `docs/DEPENDENCIES.md` current. Run
`node scripts/dependency-inventory.mjs --check` and the tests under `scripts/*.test.*`.
Report untested systems explicitly. A successful build is not proof of a successful install.

依赖升级需同步依赖清单并运行上述校验及 scripts 下的测试。明确写出未验证系统；
构建成功不代表安装成功。当前没有响应时限或自动合并承诺。

Contributions are licensed under Apache-2.0. Third-party dependencies retain their own licenses.

贡献采用 Apache-2.0；第三方依赖继续适用各自许可证。
