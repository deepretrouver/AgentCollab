# Hub 管理员初始化

首个管理员通过 Hub 本地命令创建。密码只从环境变量读取，不作为命令行参数传递。

```bash
read -s AGENTCOLLAB_ADMIN_PASSWORD
export AGENTCOLLAB_ADMIN_PASSWORD

~/.agentcollab/bin/agentcollab-hub admin create \
  --config /srv/agentcollab/config/hub.yaml \
  --username owner \
  --display-name "Owner"

unset AGENTCOLLAB_ADMIN_PASSWORD
```

用户名可使用中文、英文、数字和常用符号（最长 64 个字符，首尾空格会自动忽略）；密码可由用户自由设置（但不能为空）。密码使用 Argon2id 保存；Session 和邀请只保存令牌 SHA-256，不保存明文。

管理员创建命令只用于建立首个 Hub 管理员，且会获取单 Hub 锁；Hub 已运行时命令会拒绝离线修改数据库。

后续不需要预先写入多个管理员账号：让对方先在登录页创建普通账号，再由任一 Hub 管理员登录 Web 界面，打开左下角个人资料，选择“管理 Hub 账号权限”，即可把既有账号设为或移出 `Hub 管理员`。

`Hub 管理员`只管理 Hub 账号角色，不会自动加入、读取或修改任何工作区、群聊、Agent、Skill、设备或本地电脑。工作区 Owner / 工作区管理员、群主 / 群管理员由各工作区和群详情分别管理。

## Agent 适配目录发布

Hub 的 `/downloads/agent-catalog` 是所有 Client 的终端适配目录来源。未发布自定义目录时，Server 返回随版本内置的安全默认目录；团队需要新增或修正一个 CLI 时，将通过 schema 校验的完整 JSON 原子替换到：

```text
<storage_root>/artifacts/agent-catalog/catalog.json
```

无需重启 Hub。该接口每次请求都会读取有效发布文件并生成 ETag；在线 Client 至多 5 分钟内自动同步、缓存、重新扫描并在下一次心跳把新增 Agent 报告给 Hub。目录的 `revision` 每次发布都应递增。发布文件无效、schema 过新或声明了 Client 不认识的输出协议时，Hub 回退到内置目录，Client 也保留其最后一份有效缓存。

目录只声明可执行文件候选和参数数组，不执行 shell 字符串。普通团队不需要维护它；正式自动更新服务只需负责签发和原子替换这一份目录，无需再为每种 Agent 修改、编译和分发 Client。
