# Local source quick start

This path is for developers testing the community preview. Packaged desktop installation will have its own verified platform matrix before release.

Prerequisites: Git, Go matching `go.mod`, Node.js 22.12+ with npm, Make, rsync, and a C compiler for SQLite. These commands target a macOS/Linux shell, not Windows PowerShell. Run all commands from the repository root. This is a fresh deployment: do not overwrite an existing `config/hub.yaml` or reuse production team data for acceptance tests.

```sh
npm ci --prefix web
make web-build
make build
mkdir -p config/agentcollab-data
cp config/hub.example.yaml config/hub.yaml
```

Edit `config/hub.yaml`: set `listen_address: 127.0.0.1:8080`, `public_url: http://127.0.0.1:8080`, and replace `runner_public_urls` with the same address. The sample `storage_root: ./agentcollab-data` resolves relative to the configuration file, so it uses `config/agentcollab-data`. Keep `require_smb: false` for local testing.

Create the administrator before starting the Server. Enter a password through an environment variable; do not commit it to configuration files.

```sh
read -s AGENTCOLLAB_ADMIN_PASSWORD
export AGENTCOLLAB_ADMIN_PASSWORD
bin/agentcollab-hub admin create --config config/hub.yaml --username owner --display-name Owner
unset AGENTCOLLAB_ADMIN_PASSWORD
bin/agentcollab-hub serve --config config/hub.yaml
```

Open `http://127.0.0.1:8080` and sign in. Create a workspace and room, then use the room's invitation action to invite another account. For LAN use, change the listen address to `0.0.0.0:8080` and public URLs to the Server's reachable LAN address. Other computers must not use `127.0.0.1` to connect to this Server.

To connect native agents, use a compatible Client and its local-agent connection flow. A browser cannot scan another computer's installed tools. Each CLI must work independently before connecting it.

## Build and start the Client on macOS

Keep the source Hub running in the first terminal. In a second terminal at the same
repository root, run:

```sh
make desktop-runtime desktop-install
mkdir -p build/client-preview
AGENTCOLLAB_CLIENT_DATA_DIR="$PWD/build/client-preview" npm --prefix desktop start
```

This starts the real Client from source with a separate local profile. It does not
install the app or add login startup items. Use the same Server address as above
(or paste its invitation link), sign in to your existing team account, then use
**Connect this computer** and **Find agents**. Select installed/authenticated tools
for the workspace library, add one to the room, and mention it for a short test.
Reuse this exact profile path on later launches to keep device identity. Quit the
Client explicitly when finished; closing its window can leave it in the tray.

For a native `.app` build, `make desktop-pack` creates
`desktop/release/mac-arm64/AgentCollab.app` on Apple Silicon or
`desktop/release/mac/AgentCollab.app` on Intel. A build is not notarization or
platform acceptance. Intel and Windows remain unverified until their own checks
pass; do not treat an existing ZIP as a current download. Linux developers can
run the source Hub, but no verified desktop Client package is offered yet.

Continue with the [step-by-step user guide](USER_GUIDE.en.md) for invitations, connecting an agent and verifying its first response. The Server host needs a Client too if it contributes local agents.

## Verify from scratch

```sh
node scripts/smoke-community.mjs bin/agentcollab-hub
```

This creates an isolated temporary database, starts a real Server on a free loopback port, verifies accounts and invitations, stops the process and removes only its own temporary directory. It does not exercise the desktop UI or a real model request.
