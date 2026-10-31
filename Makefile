VERSION ?= 1.3.3
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BIN_DIR ?= bin
LDFLAGS := -X agentcollab.local/agentcollab/internal/buildinfo.Version=$(VERSION) -X agentcollab.local/agentcollab/internal/buildinfo.Commit=$(COMMIT) -X agentcollab.local/agentcollab/internal/buildinfo.BuiltAt=$(BUILT_AT)
UNAME_M := $(shell uname -m)
DESKTOP_GOARCH := $(if $(filter x86_64,$(UNAME_M)),amd64,arm64)
DESKTOP_ELECTRON_ARCH := $(if $(filter x86_64,$(UNAME_M)),x64,arm64)
DESKTOP_APP_DIR := desktop/release/$(if $(filter x64,$(DESKTOP_ELECTRON_ARCH)),mac,mac-$(DESKTOP_ELECTRON_ARCH))/AgentCollab.app
ELECTRON_DIST ?=
ELECTRON_DIST_ARG := $(if $(strip $(ELECTRON_DIST)),--config.electronDist="$(ELECTRON_DIST)",)
ELECTRON_INSTALL_ENV := $(if $(strip $(ELECTRON_DIST)),ELECTRON_SKIP_BINARY_DOWNLOAD=1,)

.PHONY: build test vet web-build runner-dist runner-packages desktop-runtime desktop-install desktop-dev desktop-pack desktop-mac desktop-win server-bundle-packages server-bundle-local server-runtime server-install server-dev server-pack server-mac dist clean

build:
	mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agentcollab-hub ./cmd/hub
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agentcollab-runner ./cmd/runner
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agentcollabctl ./cmd/agentcollabctl

test:
	go test ./...

vet:
	go vet ./...

web-build:
	npm --prefix web run build
	mkdir -p internal/httpapi/webdist
	rsync -a --delete web/dist/ internal/httpapi/webdist/

runner-dist:
	mkdir -p dist/runners/darwin-arm64 dist/runners/darwin-amd64 dist/runners/darwin-universal dist/runners/windows-amd64 dist/runners/windows-arm64 dist/runners/linux-amd64 dist/runners/linux-arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/runners/darwin-arm64/agentcollab-runner ./cmd/runner
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/runners/darwin-amd64/agentcollab-runner ./cmd/runner
	lipo -create -output dist/runners/darwin-universal/agentcollab-runner dist/runners/darwin-arm64/agentcollab-runner dist/runners/darwin-amd64/agentcollab-runner
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/runners/windows-amd64/agentcollab-runner.exe ./cmd/runner
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/runners/windows-arm64/agentcollab-runner.exe ./cmd/runner
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/runners/linux-amd64/agentcollab-runner ./cmd/runner
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/runners/linux-arm64/agentcollab-runner ./cmd/runner

runner-packages: runner-dist
	rm -rf dist/runner-package-staging dist/runner-packages
	mkdir -p dist/runner-package-staging/darwin-universal dist/runner-package-staging/linux-amd64 dist/runner-package-staging/linux-arm64 dist/runner-package-staging/windows-amd64 dist/runner-package-staging/windows-arm64 dist/runner-packages
	cp dist/runners/darwin-universal/agentcollab-runner dist/runner-package-staging/darwin-universal/
	cp dist/runners/linux-amd64/agentcollab-runner dist/runner-package-staging/linux-amd64/
	cp dist/runners/linux-arm64/agentcollab-runner dist/runner-package-staging/linux-arm64/
	cp dist/runners/windows-amd64/agentcollab-runner.exe dist/runner-package-staging/windows-amd64/
	cp dist/runners/windows-arm64/agentcollab-runner.exe dist/runner-package-staging/windows-arm64/
	cp packaging/runner/README.txt packaging/runner/install-macos.sh dist/runner-package-staging/darwin-universal/
	cp packaging/runner/README.txt packaging/runner/install-linux.sh dist/runner-package-staging/linux-amd64/
	cp packaging/runner/README.txt packaging/runner/install-linux.sh dist/runner-package-staging/linux-arm64/
	cp packaging/runner/README.txt packaging/runner/install-windows.ps1 packaging/runner/Start-AgentCollab.cmd dist/runner-package-staging/windows-amd64/
	cp packaging/runner/README.txt packaging/runner/install-windows.ps1 packaging/runner/Start-AgentCollab.cmd dist/runner-package-staging/windows-arm64/
	for directory in dist/runner-package-staging/*; do cp LICENSE docs/GO-NOTICES.md "$$directory/"; done
	chmod 755 dist/runner-package-staging/darwin-universal/install-macos.sh dist/runner-package-staging/linux-amd64/install-linux.sh dist/runner-package-staging/linux-arm64/install-linux.sh
	tar -czf dist/runner-packages/AgentCollab-Runner-macos-universal.tar.gz -C dist/runner-package-staging/darwin-universal .
	tar -czf dist/runner-packages/AgentCollab-Runner-linux-amd64.tar.gz -C dist/runner-package-staging/linux-amd64 .
	tar -czf dist/runner-packages/AgentCollab-Runner-linux-arm64.tar.gz -C dist/runner-package-staging/linux-arm64 .
	(cd dist/runner-package-staging/windows-amd64 && zip -qr ../../runner-packages/AgentCollab-Runner-windows-amd64.zip .)
	(cd dist/runner-package-staging/windows-arm64 && zip -qr ../../runner-packages/AgentCollab-Runner-windows-arm64.zip .)

desktop-runtime: web-build
	rm -rf desktop/runtime
	mkdir -p desktop/runtime
	CGO_ENABLED=0 GOOS=darwin GOARCH=$(DESKTOP_GOARCH) go build -ldflags "$(LDFLAGS)" -o desktop/runtime/agentcollab-runner ./cmd/runner

desktop-install:
	$(ELECTRON_INSTALL_ENV) npm --prefix desktop ci

desktop-dev: desktop-runtime desktop-install
	npm --prefix desktop start

desktop-pack: desktop-runtime desktop-install
	npm --prefix desktop run pack -- --$(DESKTOP_ELECTRON_ARCH) $(ELECTRON_DIST_ARG)

desktop-mac: desktop-runtime desktop-install
	npm --prefix desktop run dist:mac -- --$(DESKTOP_ELECTRON_ARCH) $(ELECTRON_DIST_ARG)

server-runtime: web-build
	rm -rf server-desktop/runtime
	mkdir -p server-desktop/runtime
	go build -ldflags "$(LDFLAGS)" -o server-desktop/runtime/agentcollab-hub ./cmd/hub

server-bundle-packages: desktop-mac runner-packages
	rm -rf server-desktop/client-packages server-desktop/runner-packages
	mkdir -p server-desktop/client-packages server-desktop/runner-packages
	ditto -c -k --sequesterRsrc --keepParent "$(DESKTOP_APP_DIR)" server-desktop/client-packages/AgentCollab-Client-macos-$(DESKTOP_GOARCH).zip
	cp dist/runner-packages/* server-desktop/runner-packages/

# Fast local acceptance package: creates directly installable .app bundles and
# a zipped Client without waiting for DMG helper downloads. Formal releases can
# still use server-mac after the local first-run flow is accepted.
server-bundle-local: desktop-pack runner-packages
	rm -rf server-desktop/client-packages server-desktop/runner-packages
	mkdir -p server-desktop/client-packages server-desktop/runner-packages
	ditto -c -k --sequesterRsrc --keepParent "$(DESKTOP_APP_DIR)" server-desktop/client-packages/AgentCollab-Client-macos-$(DESKTOP_GOARCH).zip
	cp dist/runner-packages/* server-desktop/runner-packages/

server-install:
	$(ELECTRON_INSTALL_ENV) npm --prefix server-desktop ci

server-dev: server-runtime server-install
	npm --prefix server-desktop start

server-pack: server-bundle-local server-runtime server-install
	npm --prefix server-desktop run pack -- --$(DESKTOP_ELECTRON_ARCH) $(ELECTRON_DIST_ARG)

server-mac: server-bundle-packages server-runtime server-install
	npm --prefix server-desktop run dist:mac -- $(ELECTRON_DIST_ARG)

# Run this target on a Windows build machine so sqlite3 and Electron are
# compiled and packaged natively for Windows.
desktop-win: web-build desktop-install
	powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build-desktop-windows.ps1

dist: build runner-packages
	mkdir -p dist/local
	cp $(BIN_DIR)/agentcollab-hub $(BIN_DIR)/agentcollab-runner $(BIN_DIR)/agentcollabctl dist/local/

clean:
	rm -rf $(BIN_DIR) dist desktop/runtime desktop/release server-desktop/runtime server-desktop/release
