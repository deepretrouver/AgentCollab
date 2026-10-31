package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"agentcollab.local/agentcollab/internal/buildinfo"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/runnerclient"
)

func main() {
	if len(os.Args) == 1 {
		os.Exit(runClient())
	}
	if len(os.Args) > 1 && os.Args[1] == "pair" {
		os.Exit(runPair(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		os.Exit(runSetup(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "connect" {
		os.Exit(runConnect(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "enroll" {
		os.Exit(runEnroll(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		os.Exit(runServe(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(buildinfo.Current("agentcollab-runner"))
		return
	}
	fmt.Println(buildinfo.Current("agentcollab-runner"))
}

// runClient is the double-click / no-argument entry point for every desktop
// platform. A pairing code is a one-time enrollment credential, not a login
// credential: once the local configuration and device token exist, opening
// the Client must simply restore the saved Hub connection.
func runClient() int {
	configPath := defaultRunnerConfigPath()
	if hasSavedRunnerRegistration(configPath) {
		return runServe([]string{"--config", configPath})
	}
	return runPair(nil)
}

func hasSavedRunnerRegistration(configPath string) bool {
	cfg, err := config.LoadRunner(configPath)
	if err != nil {
		return false
	}
	info, err := os.Stat(cfg.CredentialsFile)
	return err == nil && !info.IsDir()
}

func runConnect(args []string) int {
	flags := flag.NewFlagSet("connect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", defaultRunnerConfigPath(), "path to create or update Runner YAML configuration")
	hubURL := flags.String("hub-url", "", "Hub URL reachable from this computer")
	deviceName := flags.String("device-name", "", "device display name")
	inviteCodeStdin := flags.Bool("invite-code-stdin", false, "read a one-time Runner invite code from stdin")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*hubURL) == "" || !*inviteCodeStdin {
		fmt.Fprintln(os.Stderr, "connect: --hub-url and --invite-code-stdin are required")
		return 2
	}
	cfg, err := prepareRunnerConfig(*configPath, *hubURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		return 1
	}
	secretBytes, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: read invitation code: %v\n", err)
		return 1
	}
	name := strings.TrimSpace(*deviceName)
	if name == "" {
		name, _ = os.Hostname()
	}
	credentials, err := runnerclient.EnrollWithInvite(context.Background(), cfg, strings.TrimSpace(string(secretBytes)), name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: enroll Runner: %v\n", err)
		return 1
	}
	fmt.Printf("runner_connected=%s device=%s agents=%d config=%s\n", credentials.RunnerID, credentials.DeviceID, len(credentials.Agents), *configPath)
	return 0
}

// runSetup is the recipient-friendly entry point. It keeps the install-time
// flow interactive so the recipient only needs the Hub URL and one-time code.
func runSetup(args []string) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", defaultRunnerConfigPath(), "path to create or update Runner YAML configuration")
	hubURL := flags.String("hub-url", "", "Hub URL reachable from this computer; prompted when omitted")
	deviceName := flags.String("device-name", "", "device display name; prompted when omitted")
	inviteCode := flags.String("invite-code", "", "one-time Runner invite code; prompted when omitted")
	noServe := flags.Bool("no-serve", false, "enroll and exit without starting the Runner")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	reader := bufio.NewReader(os.Stdin)
	var err error
	if strings.TrimSpace(*hubURL) == "" {
		*hubURL, err = promptSetupValue(reader, "Hub 地址（例如 http://192.168.1.20:8080）", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "setup: read Hub address: %v\n", err)
			return 1
		}
	}
	if strings.TrimSpace(*inviteCode) == "" {
		*inviteCode, err = promptSetupValue(reader, "一次性接入码", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "setup: read invite code: %v\n", err)
			return 1
		}
	}
	if strings.TrimSpace(*hubURL) == "" || strings.TrimSpace(*inviteCode) == "" {
		fmt.Fprintln(os.Stderr, "setup: Hub 地址和一次性接入码不能为空")
		return 2
	}
	if strings.TrimSpace(*deviceName) == "" {
		*deviceName, err = promptSetupValue(reader, "这台电脑的显示名称", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "setup: read device name: %v\n", err)
			return 1
		}
	}
	if strings.TrimSpace(*deviceName) == "" {
		*deviceName, _ = os.Hostname()
	}
	cfg, err := prepareRunnerConfig(*configPath, *hubURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup: %v\n", err)
		return 1
	}
	if err := enrollRunner(cfg, *inviteCode, *deviceName, "setup"); err != nil {
		return 1
	}
	fmt.Printf("\n连接成功。请保持这个窗口打开；关闭窗口会停止这台电脑上的 Runner。\n" +
		"平台管理员接下来应点击‘扫描新增’，选择本机 Agent 注册，再把它加入群聊。\n")
	if *noServe {
		return 0
	}
	return runServe([]string{"--config", *configPath})
}

func promptSetupValue(reader *bufio.Reader, label, fallback string) (string, error) {
	if fallback != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, fallback)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

func prepareRunnerConfig(configPath, hubURL string) (config.Runner, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Runner{}, fmt.Errorf("find user home: %w", err)
	}
	absoluteConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return config.Runner{}, fmt.Errorf("resolve Runner config path: %w", err)
	}
	// New installations keep state beside their configuration. This isolates
	// release namespaces and test instances; existing saved paths stay intact.
	localDir := filepath.Join(filepath.Dir(absoluteConfigPath), "runner")
	cfg := config.Runner{
		HubURL: hubURL, LocalDir: localDir,
		CredentialsFile: filepath.Join(localDir, "device.json"), MaxConcurrentTasks: 6,
		Resources: []config.RunnerResource{{Name: "user-home", Path: home, Description: "本机用户目录中的文件、项目、数据库和已登录应用数据", Access: "read_write"}},
	}
	if _, statErr := os.Stat(configPath); statErr == nil {
		cfg, err = config.LoadRunner(configPath)
		if err != nil {
			return config.Runner{}, err
		}
		cfg.HubURL = hubURL
		if len(cfg.Resources) == 0 {
			cfg.Resources = []config.RunnerResource{{Name: "user-home", Path: home, Description: "本机用户目录中的文件、项目、数据库和已登录应用数据", Access: "read_write"}}
		}
	} else if !os.IsNotExist(statErr) {
		return config.Runner{}, fmt.Errorf("inspect Runner config: %w", statErr)
	}
	if err := config.SaveRunner(configPath, cfg); err != nil {
		return config.Runner{}, fmt.Errorf("save Runner config: %w", err)
	}
	return cfg, nil
}

func enrollRunner(cfg config.Runner, secret, deviceName, commandName string) error {
	credentials, err := runnerclient.EnrollWithInvite(context.Background(), cfg, strings.TrimSpace(secret), deviceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: enroll Runner: %v\n", commandName, err)
		return err
	}
	fmt.Printf("runner_connected=%s device=%s agents=%d config=%s\n", credentials.RunnerID, credentials.DeviceID, len(credentials.Agents), cfg.CredentialsFile)
	return nil
}

func defaultRunnerConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "runner.yaml"
	}
	return filepath.Join(home, ".agentcollab", "runner.yaml")
}

func runEnroll(args []string) int {
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	configPath := flags.String("config", "config/runner.yaml", "path to Runner YAML configuration")
	workspaceID := flags.String("workspace", "", "workspace ID")
	username := flags.String("username", "owner", "Hub username")
	deviceName := flags.String("device-name", "", "device display name")
	passwordStdin := flags.Bool("password-stdin", false, "read Hub password from stdin")
	inviteCodeStdin := flags.Bool("invite-code-stdin", false, "read a one-time Runner invite code from stdin")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *inviteCodeStdin == *passwordStdin {
		fmt.Fprintln(os.Stderr, "enroll: choose exactly one of --invite-code-stdin or --password-stdin")
		return 2
	}
	if *passwordStdin && *workspaceID == "" {
		fmt.Fprintln(os.Stderr, "enroll: --workspace is required with --password-stdin")
		return 2
	}
	cfg, err := config.LoadRunner(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll: %v\n", err)
		return 1
	}
	secretBytes, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll: read enrollment secret: %v\n", err)
		return 1
	}
	name := strings.TrimSpace(*deviceName)
	if name == "" {
		name, _ = os.Hostname()
	}
	secret := strings.TrimSpace(string(secretBytes))
	var credentials runnerclient.Credentials
	if *inviteCodeStdin {
		credentials, err = runnerclient.EnrollWithInvite(context.Background(), cfg, secret, name)
	} else {
		credentials, err = runnerclient.Enroll(context.Background(), cfg, *workspaceID, *username, secret, name)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "enroll: %v\n", err)
		return 1
	}
	fmt.Printf("runner_enrolled=%s device=%s agents=%d\n", credentials.RunnerID, credentials.DeviceID, len(credentials.Agents))
	return 0
}

func runServe(args []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "config/runner.yaml", "path to Runner YAML configuration")
	once := flags.Bool("once", false, "poll at most one task and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.LoadRunner(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	client, err := runnerclient.New(cfg, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := client.Serve(ctx, *once); err != nil && !errorsIsContext(err) {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}

func errorsIsContext(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
