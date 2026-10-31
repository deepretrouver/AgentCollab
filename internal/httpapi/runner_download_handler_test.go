package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentcollab.local/agentcollab/internal/config"
)

func publishTestPackage(t *testing.T, root, directory, filename, body string) {
	t.Helper()
	packageRoot := filepath.Join(root, "artifacts", directory)
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, filename), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClientDownloadPageListsOnlyGraphicalInstallers(t *testing.T) {
	storageRoot := t.TempDir()
	filename := clientPackages["darwin-arm64"]
	publishTestPackage(t, storageRoot, "client-packages", filename, "mac-desktop-client")
	windowsFilename := clientPackages["windows-amd64"]
	publishTestPackage(t, storageRoot, "client-packages", windowsFilename, "windows-desktop-client")

	request := httptest.NewRequest(http.MethodGet, "/downloads/client", nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Apple Silicon Mac OS X)")
	response := httptest.NewRecorder()
	NewClientDownloadHandler(storageRoot, "0.4.4").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("content type=%q", contentType)
	}
	for _, expected := range []string{"下载 Client", "Download Client", "0.4.4", filename, windowsFilename, "Windows 10/11（x64）", "运行其中的 .exe 安装程序", "run the .exe installer", "Applications", "关于本机"} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("download page does not contain %q: %s", expected, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), "双击 AgentCollab.exe") {
		t.Fatal("download page still assumes a portable Windows executable")
	}
	if strings.Contains(response.Body.String(), "AgentCollab-Runner") {
		t.Fatalf("graphical Client page exposed the legacy Runner: %s", response.Body.String())
	}
}

func TestClientDownloadServesExplicitGraphicalInstaller(t *testing.T) {
	storageRoot := t.TempDir()
	filename := clientPackages["windows-amd64"]
	publishTestPackage(t, storageRoot, "client-packages", filename, "windows-desktop-client")

	request := httptest.NewRequest(http.MethodGet, "/downloads/client?platform=windows-amd64", nil)
	response := httptest.NewRecorder()
	NewClientDownloadHandler(storageRoot).ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "windows-desktop-client" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="`+filename+`"` {
		t.Fatalf("content disposition=%q", got)
	}
}

func TestRunnerDownloadRemainsASeparateCompatibilityRoute(t *testing.T) {
	storageRoot := t.TempDir()
	filename := runnerPackages["linux-arm64"]
	publishTestPackage(t, storageRoot, "runner-packages", filename, "linux-runner")

	request := httptest.NewRequest(http.MethodGet, "/downloads/runner?platform=linux-arm64", nil)
	response := httptest.NewRecorder()
	NewRunnerDownloadHandler(storageRoot).ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "linux-runner" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestClientDownloadRejectsUnknownPlatform(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/downloads/client?platform=browser", nil)
	response := httptest.NewRecorder()
	NewClientDownloadHandler(t.TempDir()).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestClientDownloadManifestPublishesVersionHashesAndURLs(t *testing.T) {
	storageRoot := t.TempDir()
	filename := clientPackages["windows-amd64"]
	publishTestPackage(t, storageRoot, "client-packages", filename, "windows-client")
	request := httptest.NewRequest(http.MethodGet, "/downloads/client/manifest", nil)
	response := httptest.NewRecorder()
	NewClientDownloadHandler(storageRoot, "0.4.4").ServeManifest(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"version":"0.4.4"`) || !strings.Contains(body, `"platform":"windows-amd64"`) || !strings.Contains(body, `"sha256":"`) || !strings.Contains(body, `"download_url":"/downloads/client?platform=windows-amd64"`) {
		t.Fatalf("manifest=%s", body)
	}
}

func TestRouterPublishesSeparateClientAndRunnerDownloads(t *testing.T) {
	storageRoot := t.TempDir()
	publishTestPackage(t, storageRoot, "client-packages", clientPackages["darwin-arm64"], "client")
	publishTestPackage(t, storageRoot, "runner-packages", runnerPackages["darwin-universal"], "runner")

	mux := http.NewServeMux()
	clients := NewClientDownloadHandler(storageRoot, "0.4.4")
	runners := NewRunnerDownloadHandler(storageRoot, "0.4.4")
	mux.Handle("GET /downloads/client", clients)
	mux.HandleFunc("GET /downloads/client/manifest", clients.ServeManifest)
	mux.Handle("GET /downloads/runner", runners)
	mux.HandleFunc("GET /downloads/runner/manifest", runners.ServeManifest)
	NewHubInfoHandler(config.Hub{HubName: "Office", PublicURL: "http://hub.office:8080"}, "0.4.4").Register(mux)

	for _, target := range []string{"/downloads/client?platform=darwin-arm64", "/downloads/client/manifest", "/downloads/runner?platform=darwin-universal", "/api/v1/hub/info"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
}
