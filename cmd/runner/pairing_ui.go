package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"agentcollab.local/agentcollab/internal/runnerclient"
)

type pairingInput struct {
	HubURL     string
	InviteCode string
	DeviceName string
}

type pairingResult struct {
	RunnerID   string
	DeviceID   string
	AgentCount int
}

type pairingEnrollFunc func(context.Context, pairingInput) (pairingResult, error)

type pairingPageData struct {
	HubURL     string
	DeviceName string
	Nonce      string
	Error      string
	Success    bool
	AgentCount int
}

type pairingHandler struct {
	defaultHubURL     string
	defaultDeviceName string
	nonce             string
	enroll            pairingEnrollFunc
	onPaired          func(pairingResult)
	mu                sync.Mutex
	busy              bool
	paired            bool
}

func runPair(args []string) int {
	flags := flag.NewFlagSet("pair", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", defaultRunnerConfigPath(), "path to create or update Runner YAML configuration")
	hubURL := flags.String("hub-url", "", "Hub URL to prefill in the browser pairing page")
	deviceName := flags.String("device-name", "", "device display name to prefill")
	noOpen := flags.Bool("no-open", false, "do not open the browser automatically")
	noServe := flags.Bool("no-serve", false, "enroll and exit without starting the Runner")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*deviceName) == "" {
		*deviceName, _ = os.Hostname()
	}
	nonce, err := newPairingNonce()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pair: create local security token: %v\n", err)
		return 1
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pair: start local pairing page: %v\n", err)
		return 1
	}
	defer listener.Close()

	paired := make(chan pairingResult, 1)
	handler := newPairingHandler(strings.TrimSpace(*hubURL), strings.TrimSpace(*deviceName), nonce, func(ctx context.Context, input pairingInput) (pairingResult, error) {
		cfg, err := prepareRunnerConfig(*configPath, input.HubURL)
		if err != nil {
			return pairingResult{}, err
		}
		credentials, err := runnerclient.EnrollWithInvite(ctx, cfg, input.InviteCode, input.DeviceName)
		if err != nil {
			return pairingResult{}, err
		}
		return pairingResult{RunnerID: string(credentials.RunnerID), DeviceID: string(credentials.DeviceID), AgentCount: len(credentials.Agents)}, nil
	}, func(result pairingResult) {
		select {
		case paired <- result:
		default:
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	pairingURL := "http://" + listener.Addr().String() + "/"
	fmt.Printf("AgentCollab 配对页面：%s\n", pairingURL)
	fmt.Println("浏览器未自动打开时，请复制上面的本机地址。配对前请保持此窗口打开。")
	if !*noOpen {
		if err := openBrowser(pairingURL); err != nil {
			fmt.Fprintf(os.Stderr, "pair: cannot open browser automatically: %v\n", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var result pairingResult
	select {
	case result = <-paired:
	case err := <-serverErrors:
		fmt.Fprintf(os.Stderr, "pair: local pairing page stopped: %v\n", err)
		return 1
	case <-ctx.Done():
		shutdownLocalPairingServer(server)
		return 0
	}
	shutdownLocalPairingServer(server)
	fmt.Printf("runner_connected=%s device=%s agents=%d config=%s\n", result.RunnerID, result.DeviceID, result.AgentCount, *configPath)
	fmt.Println("连接成功。关闭配对网页后，请回到 Client 启动窗口。新版 Windows 和 macOS 安装包会自动转入后台；Linux 请保持 Client 运行。")
	fmt.Println("回到 AgentCollab 点击“扫描新增”，注册这台电脑上的 Agent，再加入群聊。")
	if *noServe {
		return 0
	}
	return runServe([]string{"--config", *configPath})
}

func newPairingHandler(defaultHubURL, defaultDeviceName, nonce string, enroll pairingEnrollFunc, onPaired func(pairingResult)) http.Handler {
	return &pairingHandler{defaultHubURL: defaultHubURL, defaultDeviceName: defaultDeviceName, nonce: nonce, enroll: enroll, onPaired: onPaired}
}

func (h *pairingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setPairingSecurityHeaders(w)
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.render(w, http.StatusOK, pairingPageData{HubURL: h.defaultHubURL, DeviceName: h.defaultDeviceName, Nonce: h.nonce})
	case http.MethodPost:
		h.handlePost(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *pairingHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		h.render(w, http.StatusBadRequest, pairingPageData{HubURL: h.defaultHubURL, DeviceName: h.defaultDeviceName, Nonce: h.nonce, Error: "提交内容无效，请重新填写。"})
		return
	}
	hubURL := strings.TrimSpace(r.FormValue("hub_url"))
	deviceName := strings.TrimSpace(r.FormValue("device_name"))
	inviteCode := strings.TrimSpace(r.FormValue("invite_code"))
	data := pairingPageData{HubURL: hubURL, DeviceName: deviceName, Nonce: h.nonce}
	if r.FormValue("nonce") != h.nonce {
		data.Error = "配对页面已失效，请关闭后重新运行 AgentCollab Client。"
		h.render(w, http.StatusBadRequest, data)
		return
	}
	if err := validatePairingInput(pairingInput{HubURL: hubURL, InviteCode: inviteCode, DeviceName: deviceName}); err != nil {
		data.Error = err.Error()
		h.render(w, http.StatusBadRequest, data)
		return
	}
	h.mu.Lock()
	if h.paired {
		h.mu.Unlock()
		data.Success = true
		h.render(w, http.StatusOK, data)
		return
	}
	if h.busy {
		h.mu.Unlock()
		data.Error = "正在连接，请不要重复提交。"
		h.render(w, http.StatusConflict, data)
		return
	}
	h.busy = true
	h.mu.Unlock()

	result, err := h.enroll(r.Context(), pairingInput{HubURL: hubURL, InviteCode: inviteCode, DeviceName: deviceName})
	h.mu.Lock()
	h.busy = false
	if err == nil {
		h.paired = true
	}
	h.mu.Unlock()
	if err != nil {
		data.Error = "连接失败：" + err.Error()
		h.render(w, http.StatusBadGateway, data)
		return
	}
	data.Success = true
	data.AgentCount = result.AgentCount
	h.render(w, http.StatusOK, data)
	if h.onPaired != nil {
		h.onPaired(result)
	}
}

func (h *pairingHandler) render(w http.ResponseWriter, status int, data pairingPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pairingPageTemplate.Execute(w, data); err != nil {
		fmt.Fprintf(os.Stderr, "pair: render local page: %v\n", err)
	}
}

func validatePairingInput(input pairingInput) error {
	parsed, err := url.Parse(input.HubURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return fmt.Errorf("Hub 地址必须是完整的 http 或 https 地址，例如 http://192.168.1.20:8080")
	}
	if input.InviteCode == "" {
		return fmt.Errorf("一次性设备配对码不能为空")
	}
	if len(input.InviteCode) > 512 {
		return fmt.Errorf("一次性设备配对码格式不正确")
	}
	if input.DeviceName == "" {
		return fmt.Errorf("设备名称不能为空")
	}
	if len([]rune(input.DeviceName)) > 120 {
		return fmt.Errorf("设备名称不能超过 120 个字符")
	}
	return nil
}

func newPairingNonce() (string, error) {
	payload := make([]byte, 24)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func setPairingSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

func shutdownLocalPairingServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("cmd", "/c", "start", "", target)
	case "darwin":
		command = exec.Command("open", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return command.Start()
}

var pairingPageTemplate = template.Must(template.New("pairing").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>连接 AgentCollab</title><style>
:root{color-scheme:dark;font-family:Inter,"PingFang SC","Microsoft YaHei",system-ui,sans-serif;background:#101722;color:#eef4ff}*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:28px;background:radial-gradient(circle at 50% 0,#243450 0,#101722 48%)}main{width:min(620px,100%);background:#182331;border:1px solid #34445a;border-radius:20px;padding:32px;box-shadow:0 24px 70px #050a1188}.brand{display:flex;align-items:center;gap:12px;font-weight:800;font-size:18px}.mark{display:grid;place-items:center;width:38px;height:38px;border-radius:11px;background:#dceaff;color:#152238}h1{font-size:30px;margin:28px 0 8px}p{color:#aebbd0;line-height:1.7;margin:0 0 22px}.notice,.error,.success{border-radius:12px;padding:14px 16px;margin:18px 0;line-height:1.55}.notice{background:#202e40;color:#c8d5e8}.error{background:#48262c;border:1px solid #8b414d;color:#ffd9df}.success{background:#183b31;border:1px solid #357662;color:#d6fff0}label{display:block;margin:18px 0 8px;font-weight:700}small{display:block;color:#8796ac;font-weight:400;margin-top:5px}input{width:100%;border:1px solid #45566e;background:#111a26;color:#f4f7fb;border-radius:10px;padding:13px 14px;font:inherit;outline:none}input:focus{border-color:#91b7ee;box-shadow:0 0 0 3px #6094dd33}button{width:100%;border:0;border-radius:11px;margin-top:24px;padding:14px;background:#e2edff;color:#142139;font:inherit;font-weight:800;cursor:pointer}button:hover{background:#fff}.foot{font-size:13px;margin:18px 0 0}.ok{font-size:42px;margin:20px 0 8px}
</style></head><body><main><div class="brand"><span class="mark">AC</span>AgentCollab Client</div>
{{if .Success}}<div class="ok">✓</div><h1>这台电脑已连接</h1><div class="success">配对成功，已检测并登记 {{.AgentCount}} 个本机 Agent 候选。</div><p>请返回 AgentCollab 群聊，点击“扫描新增”选择要注册的 Agent。这个页面可以关闭。新版 Windows 和 macOS 安装包会自动转入后台；Linux 请保持 Client 运行。</p>
{{else}}<h1>连接这台电脑</h1><p>填写协作平台管理员提供的信息。连接后，群聊中被点名的 Agent 可以在这台电脑上调用本地 CLI、Skills、文件和其他资源。</p><div class="notice">当前测试版按这台电脑的登录用户权限执行任务。请只连接你信任的 Hub。</div>{{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}
<form method="post" autocomplete="off"><input type="hidden" name="nonce" value="{{.Nonce}}"><label for="hub">Hub 地址<small>例如 http://192.168.1.20:8080</small></label><input id="hub" name="hub_url" type="url" required value="{{.HubURL}}" placeholder="http://192.168.1.20:8080"><label for="code">一次性设备配对码<small>在 AgentCollab 右侧“接入我的电脑 / Agent”中生成</small></label><input id="code" name="invite_code" type="password" required value="" autocomplete="new-password"><label for="device">设备名称<small>群聊成员会用这个名称识别任务实际在哪台电脑执行</small></label><input id="device" name="device_name" required maxlength="120" value="{{.DeviceName}}"><button type="submit">连接并启动 Client</button></form><p class="foot">配对页只在本机 127.0.0.1 临时开放；一次性码不会保存在浏览器页面中。</p>{{end}}
</main></body></html>`))
