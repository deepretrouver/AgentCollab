# Your first team collaboration

[简体中文](USER_GUIDE.zh-CN.md) · [Source setup](QUICKSTART.en.md)

This guide describes the current source UI. Community installers are still undergoing acceptance testing; this is not a claim that every OS is verified. Some screens still have Chinese labels, included below where useful.

## Three entry points

| Entry | Who needs it | Purpose |
| --- | --- | --- |
| Server | One designated team host | Stores accounts, conversations, projects and files; provides the team address |
| Browser | People discussing, assigning or reviewing work | Open an invitation and sign in or register; no Server installation needed |
| Client | Members contributing agents from their computer | Connect the computer and discover installed native CLIs |

Even on the Server's own computer, use a Client to contribute local agents. Do not ask every member to install the Server. Login startup is not required; manually start the applications you need after restarting your computer.

## 1. Create the team workspace

1. Complete Server onboarding and start the service. For a source deployment, follow the quick start to create an administrator and start the Hub.
2. Open the team address and sign in. Beside **Workspaces**, choose **+ / Create workspace** and name the project or team.
3. Select the workspace, then **+ / Create room** beside **Rooms**. You can start without agents.
4. In the right member panel, choose **+ / 邀请与接入** (invitations and connections), then **邀请协作者 → 生成邀请链接** (invite collaborators → generate invitation). Send the complete link.

The team address opens the service. An invitation additionally grants entry to its designated workspace and room. Never publish real invitation links, administrator passwords or device credentials.

LAN members need a reachable LAN address. `127.0.0.1` refers to each visitor's own computer, not your Server. The service becomes unavailable when the Server host sleeps or the Server exits.

## 2. Join as a person

1. Open the invitation. Create your own account or select **Sign in**. Do not share the administrator account.
2. Confirm the intended workspace and room appear, then send a regular message.
3. Once joined, use the team address to sign in again. An expired invitation is not an expired account; invitations are not daily login credentials.

Browser users can participate, view permitted material and call connected, authorized agents. They cannot directly scan installed programs on their computer.

## 3. Bring your agents

1. Confirm the relevant CLI starts and is authenticated independently. Being signed in to a vendor's desktop app does not necessarily authenticate its CLI.
2. Open the full team invitation in the Client and sign in to your own account. In the room, open **邀请与接入 → 添加我的 Agent → 打开本机 Agent** (invitations and connections → add my agents → open local agents).
3. If unconnected, choose **Connect this computer and scan**. If offline, choose **Reconnect and scan**. If linked to a different account, verify ownership before transferring it.
4. Select discovered agents and choose **Add N agents**. This adds them to the **workspace agent library**, not every room.
5. In the room's available agents list, choose **+** and verify the agent appears among room members.
6. Use **我的 Agent 设置 / 权限设置** (my agent settings / permissions) to select visibility, invocation access and shared skills. Visibility alone does not grant invocation permission.

Start with one CLI known to work. Discovery does not prove authentication; an online indicator does not guarantee task success.

## 4. Verify a real response

Select one agent in the room and send:

> Do not call tools or read files. Reply only with: TEAM_CHECK_001

Expand the topic. Check for the exact text in the agent's response and inspect **执行凭证** (execution evidence), when provided. An error, a login request or a progress card without a response does not pass this check.

Ask a second member to invoke the same agent to test sharing. If it fails, check room membership, the owner's invocation policy, whether the owning computer's Client is running, and CLI authentication. Do not broaden whole-computer permissions to bypass diagnosis.

Use execution evidence and local CLI sessions/logs to verify invocation. Not every vendor shares task history between its CLI and desktop app; do not assume every task appears in the vendor's GUI.

## 5. Move from discussion to project work

- Use regular messages to discuss and mentions to assign agent work. Validate one agent before trying several.
- Define the project objective, weighted steps and owners. Associate a new conversation or task with its step. General chat does not automatically advance project progress.
- Knowledge libraries hold references; artifacts hold deliveries. Sharing material does not mean every file's full content is inserted into every model request.
- Use task branches for changes that need review. Inspect differences before merging. An agent saying “done”, a delivered task and an accepted project step are distinct states.
- Rehearse with fictional data and small, non-sensitive files. Do not upload `.env` files, tokens, personal databases or your entire home directory.

## Understand connection indicators

| Indicator | Meaning / next action |
| --- | --- |
| Messages live | The page's messaging connection works; this does not prove every agent is online |
| Messages auto-syncing (previously polling sync) | The page periodically fetches messages; check device and agent health separately |
| Device or agent offline | Check power, network reachability and whether that computer's Client is running |
| Task asks for login | Check that CLI's authentication on its own computer; do not recreate your platform account |
| Empty discovery | Check account binding, CLI installation and executable discovery; do not repeatedly create workspaces |

Refreshing a browser cannot remotely start an exited Client. Closing a window and quitting the application are different; inspect application status rather than assuming that a hidden window means the background service stopped.
