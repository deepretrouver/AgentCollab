package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PackageDownloadHandler publishes versioned, secret-free installers from the
// Hub storage root. The normal Client route serves the graphical desktop app;
// the Runner route is retained only for headless and recovery installations.
type PackageDownloadHandler struct {
	packageRoot string
	version     string
	route       string
	product     string
	packages    map[string]string
	landingPage bool
}

func NewClientDownloadHandler(storageRoot string, versions ...string) *PackageDownloadHandler {
	return newPackageDownloadHandler(
		filepath.Join(storageRoot, "artifacts", "client-packages"),
		"/downloads/client",
		"AgentCollab Client",
		clientPackages,
		true,
		versions...,
	)
}

func NewRunnerDownloadHandler(storageRoot string, versions ...string) *PackageDownloadHandler {
	return newPackageDownloadHandler(
		filepath.Join(storageRoot, "artifacts", "runner-packages"),
		"/downloads/runner",
		"AgentCollab Runner",
		runnerPackages,
		false,
		versions...,
	)
}

func newPackageDownloadHandler(packageRoot, route, product string, packages map[string]string, landingPage bool, versions ...string) *PackageDownloadHandler {
	version := "dev"
	if len(versions) > 0 && strings.TrimSpace(versions[0]) != "" {
		version = strings.TrimSpace(versions[0])
	}
	return &PackageDownloadHandler{
		packageRoot: packageRoot,
		version:     version,
		route:       route,
		product:     product,
		packages:    packages,
		landingPage: landingPage,
	}
}

type releasePackage struct {
	Platform    string `json:"platform"`
	Label       string `json:"label"`
	Filename    string `json:"filename"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"`
}

type releaseManifest struct {
	Version     string           `json:"version"`
	PublishedAt *time.Time       `json:"published_at,omitempty"`
	Packages    []releasePackage `json:"packages"`
}

func (h *PackageDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.landingPage && strings.TrimSpace(r.URL.Query().Get("platform")) == "" {
		h.serveLandingPage(w, r)
		return
	}
	filename, ok := packageForRequest(r, h.packages)
	if !ok {
		http.Error(w, "unsupported platform; open the download page and choose an available installer", http.StatusBadRequest)
		return
	}
	h.serveFile(w, r, filename)
}

func (h *PackageDownloadHandler) serveFile(w http.ResponseWriter, r *http.Request, filename string) {
	path := filepath.Join(h.packageRoot, filename)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.Error(w, h.product+" package is not published on this Hub yet", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", packageContentType(filename))
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	http.ServeFile(w, r, path)
}

func (h *PackageDownloadHandler) ServeManifest(w http.ResponseWriter, _ *http.Request) {
	manifest, err := h.manifest()
	if err != nil {
		http.Error(w, "cannot inspect published "+h.product+" packages", http.StatusInternalServerError)
		return
	}
	if len(manifest.Packages) == 0 {
		http.Error(w, h.product+" package is not published on this Hub yet", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, manifest)
}

func (h *PackageDownloadHandler) manifest() (releaseManifest, error) {
	platforms := make([]string, 0, len(h.packages))
	for platform := range h.packages {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	manifest := releaseManifest{Version: h.version, Packages: make([]releasePackage, 0, len(platforms))}
	var publishedAt time.Time
	for _, platform := range platforms {
		filename := h.packages[platform]
		path := filepath.Join(h.packageRoot, filename)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		hash, err := fileSHA256(path)
		if err != nil {
			return releaseManifest{}, err
		}
		if info.ModTime().After(publishedAt) {
			publishedAt = info.ModTime().UTC()
		}
		manifest.Packages = append(manifest.Packages, releasePackage{
			Platform:    platform,
			Label:       platformLabel(platform),
			Filename:    filename,
			SizeBytes:   info.Size(),
			SHA256:      hash,
			DownloadURL: h.route + "?platform=" + platform,
		})
	}
	if !publishedAt.IsZero() {
		manifest.PublishedAt = &publishedAt
	}
	return manifest, nil
}

func (h *PackageDownloadHandler) serveLandingPage(w http.ResponseWriter, r *http.Request) {
	manifest, err := h.manifest()
	if err != nil {
		http.Error(w, "cannot inspect published "+h.product+" packages", http.StatusInternalServerError)
		return
	}
	if len(manifest.Packages) == 0 {
		http.Error(w, h.product+" package is not published on this Hub yet", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := clientDownloadPage.Execute(w, struct {
		Version  string
		Packages []releasePackage
	}{Version: h.version, Packages: manifest.Packages}); err != nil {
		http.Error(w, "cannot render Client download page", http.StatusInternalServerError)
	}
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func packageForRequest(r *http.Request, packages map[string]string) (string, bool) {
	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))
	if platform == "" {
		platform = detectedPlatform(r.UserAgent())
	}
	filename, ok := packages[platform]
	return filename, ok
}

func detectedPlatform(userAgent string) string {
	userAgent = strings.ToLower(userAgent)
	switch {
	case strings.Contains(userAgent, "windows"):
		return "windows-amd64"
	case strings.Contains(userAgent, "macintosh") || strings.Contains(userAgent, "mac os x"):
		return "darwin-arm64"
	case strings.Contains(userAgent, "linux"):
		if strings.Contains(userAgent, "aarch64") || strings.Contains(userAgent, "arm64") {
			return "linux-arm64"
		}
		return "linux-amd64"
	default:
		return ""
	}
}

func platformLabel(platform string) string {
	switch platform {
	case "darwin-universal":
		return "macOS（Apple 芯片与 Intel）"
	case "darwin-arm64":
		return "macOS（Apple 芯片）"
	case "darwin-amd64":
		return "macOS（Intel）"
	case "windows-amd64":
		return "Windows 10/11（x64）"
	case "windows-arm64":
		return "Windows 11（ARM64）"
	case "linux-amd64":
		return "Linux（x64）"
	case "linux-arm64":
		return "Linux（ARM64）"
	default:
		return platform
	}
}

var clientPackages = map[string]string{
	"darwin-arm64":  "AgentCollab-Client-macos-arm64.zip",
	"darwin-amd64":  "AgentCollab-Client-macos-amd64.zip",
	"windows-amd64": "AgentCollab-Client-windows-x64.zip",
}

var runnerPackages = map[string]string{
	"darwin-universal": "AgentCollab-Runner-macos-universal.tar.gz",
	"windows-amd64":    "AgentCollab-Runner-windows-amd64.zip",
	"windows-arm64":    "AgentCollab-Runner-windows-arm64.zip",
	"linux-amd64":      "AgentCollab-Runner-linux-amd64.tar.gz",
	"linux-arm64":      "AgentCollab-Runner-linux-arm64.tar.gz",
}

func packageContentType(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".dmg"):
		return "application/x-apple-diskimage"
	case strings.HasSuffix(filename, ".exe"):
		return "application/vnd.microsoft.portable-executable"
	case strings.HasSuffix(filename, ".zip"):
		return "application/zip"
	default:
		return "application/gzip"
	}
}

var clientDownloadPage = template.Must(template.New("client-download").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>下载 AgentCollab Client</title><style>
:root{color-scheme:light;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f4f7fb;color:#172335}*{box-sizing:border-box}body{margin:0;padding:48px 20px}.page{width:min(720px,100%);margin:auto}.brand{display:flex;align-items:center;gap:12px;font-weight:760}.mark{display:grid;grid-template-columns:repeat(2,7px);gap:5px;padding:10px;border-radius:12px;background:#356ae6}.mark i{width:7px;height:7px;border-radius:50%;background:white}.card{margin-top:28px;padding:32px;border:1px solid #dfe7f0;border-radius:18px;background:white;box-shadow:0 18px 45px #20365312}h1{margin:0;font-size:34px;letter-spacing:-.04em}p{color:#607086;line-height:1.7}.packages{display:grid;gap:12px;margin-top:26px}.package{display:flex;align-items:center;gap:18px;padding:16px;border:1px solid #dfe7f0;border-radius:12px}.package span{display:grid;gap:3px;flex:1}.package small{color:#7b899b}.package a{padding:10px 15px;border-radius:9px;background:#356ae6;color:white;text-decoration:none;font-weight:700}.recommended{color:#287554!important}.foot{margin-top:22px;font-size:13px}.legacy{margin-top:18px;padding-top:18px;border-top:1px solid #e5ebf2;font-size:13px}
</style></head><body><main class="page"><div class="brand"><span class="mark"><i></i><i></i><i></i><i></i></span>AgentCollab</div><section class="card">
<h1>下载 Client / <span lang="en">Download Client</span></h1>
<p>版本 / Version {{.Version}}。安装 Client 后，用自己的账号登录团队，再连接本机并扫描 Agent。<br><span lang="en">Install Client, sign in to your team, then connect this computer and discover your local Agents.</span></p>
<p>Mac 用户请在“关于本机”确认 Apple 芯片或 Intel；浏览器不一定能识别芯片类型。<br><span lang="en">On Mac, check Apple silicon or Intel in About This Mac. Browser detection may not identify your processor correctly.</span></p>
<div class="packages">{{range .Packages}}<div class="package"><span><strong>{{.Label}}</strong><small>{{.Filename}}</small>
{{if eq .Platform "windows-amd64"}}<small>下载后解压，运行其中的 .exe 安装程序，按向导完成安装。</small><small lang="en">Extract the ZIP, run the .exe installer inside, and follow the setup wizard.</small>
{{else}}<small>下载后解压，将 AgentCollab.app 拖入“应用程序”。</small><small lang="en">Extract the ZIP and move AgentCollab.app to Applications.</small>{{end}}
</span><a href="{{.DownloadURL}}">下载 / Download</a></div>{{end}}</div>
<p class="foot">普通成员无需安装 Server。更新时保留原有应用数据，不要删除账号或重新注册。<br><span lang="en">Members do not need Server. Keep existing app data when updating; do not delete or recreate your account.</span></p>
<p class="legacy">无界面 Runner 仅用于服务器或故障恢复，不是普通成员的安装入口。<br><span lang="en">The headless Runner is for server or recovery use, not the normal member installer.</span></p>
</section></main></body></html>`))
