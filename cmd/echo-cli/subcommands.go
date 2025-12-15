package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"echo-cli/internal/config"
	"echo-cli/internal/features"
	"echo-cli/internal/policy"
	"echo-cli/internal/sandbox"
)

func reviewMain(root rootArgs, args []string) {
	execMain(root, append([]string{"review"}, args...))
}

func loginMain(root rootArgs, args []string) {
	if len(args) > 0 && args[0] == "status" {
		cfg, err := config.Load("")
		if err != nil {
			log.Fatalf("failed to load credentials: %v", err)
		}
		if strings.TrimSpace(cfg.Token) == "" {
			fmt.Println("not logged in")
		} else {
			fmt.Println("token configured")
		}
		return
	}

	fs := flag.NewFlagSet("login", flag.ExitOnError)
	var withAPIKey bool
	var apiKey string
	var deviceAuth bool
	var issuer string
	var clientID string
	fs.BoolVar(&withAPIKey, "with-api-key", false, "Read the auth token from stdin")
	fs.StringVar(&apiKey, "api-key", "", "(deprecated) Use --with-api-key and pipe the value instead")
	fs.BoolVar(&deviceAuth, "device-auth", false, "Use device code login (not supported in echo-cli)")
	fs.StringVar(&issuer, "experimental_issuer", "", "Experimental issuer base URL (unused placeholder)")
	fs.StringVar(&clientID, "experimental_client-id", "", "Experimental client ID (unused placeholder)")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse login args: %v", err)
	}

	if apiKey != "" {
		log.Fatalf("The --api-key flag is no longer supported. Pipe the token instead, e.g. `printenv ANTHROPIC_AUTH_TOKEN | echo-cli login --with-api-key`.")
	}
	if deviceAuth {
		log.Fatalf("Device auth is not implemented in echo-cli; please use --with-api-key instead.")
	}
	_ = issuer
	_ = clientID

	var key string
	switch {
	case withAPIKey:
		key = readTokenFromStdin()
	case strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN")) != "":
		key = strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN"))
	case strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "":
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	default:
		key = promptToken()
	}
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	cfg.Token = key
	if err := config.Save("", cfg); err != nil {
		log.Fatalf("failed to save token: %v", err)
	}
	fmt.Println("Token saved.")
}

func logoutMain(root rootArgs, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	cfg.Token = ""
	if err := config.Save("", cfg); err != nil {
		log.Fatalf("failed to clear stored token: %v", err)
	}
	fmt.Println("Logged out and cleared stored token.")
}

func applyMain(root rootArgs, args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	var cfgPath string
	var patchPath string
	var workdir string
	var sandboxMode string
	fs.StringVar(&cfgPath, "config", "", "Path to config file (default ~/.echo/config.toml)")
	fs.StringVar(&patchPath, "patch", "", "Path to unified diff (default: patch.diff or first arg)")
	fs.StringVar(&workdir, "cd", "", "Working directory to apply the patch in")
	fs.StringVar(&sandboxMode, "sandbox", "", "Sandbox mode (read-only|workspace-write|danger-full-access)")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse apply args: %v", err)
	}
	if patchPath == "" {
		if fs.NArg() > 0 {
			patchPath = fs.Arg(0)
		} else {
			patchPath = "patch.diff"
		}
	}

	data, err := os.ReadFile(patchPath)
	if err != nil {
		log.Fatalf("failed to read patch %s: %v", patchPath, err)
	}

	_ = cfgPath
	rt := applyRuntimeKVOverrides(defaultRuntimeConfig(), prependOverrides(root.overrides, nil))
	if sandboxMode != "" {
		rt.SandboxMode = sandboxMode
	}
	workdir = resolveWorkdir(workdir)
	roots := []string{workdir}
	runner := sandbox.NewRunner(rt.SandboxMode, roots...)
	if err := runner.ApplyPatch(context.Background(), workdir, string(data)); err != nil {
		log.Fatalf("apply patch failed: %v", err)
	}
	fmt.Printf("Applied patch from %s\n", patchPath)
}

func sandboxMain(root rootArgs, args []string) {
	fs := flag.NewFlagSet("sandbox", flag.ExitOnError)
	var sandboxMode string
	var workdir string
	var cfgPath string
	var addDirs stringSlice
	fs.StringVar(&sandboxMode, "sandbox", "", "Sandbox mode (read-only|workspace-write|danger-full-access)")
	fs.StringVar(&sandboxMode, "s", "", "Alias for --sandbox")
	fs.StringVar(&workdir, "cd", "", "Working directory")
	fs.StringVar(&cfgPath, "config", "", "Path to config file (default ~/.echo/config.toml)")
	fs.Var(&addDirs, "add-dir", "Additional allowed workspace root (repeatable)")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse sandbox args: %v", err)
	}
	if fs.NArg() == 0 {
		log.Fatalf("sandbox requires a command to run")
	}
	command := strings.Join(fs.Args(), " ")

	_ = cfgPath
	rt := applyRuntimeKVOverrides(defaultRuntimeConfig(), prependOverrides(root.overrides, nil))
	if sandboxMode != "" {
		rt.SandboxMode = sandboxMode
	}
	workdir = resolveWorkdir(workdir)
	roots := append([]string{}, workdir)
	roots = append(roots, []string(addDirs)...)
	runner := sandbox.NewRunner(rt.SandboxMode, roots...)
	out, code, err := runner.RunCommand(context.Background(), workdir, command)
	if err != nil {
		log.Fatalf("sandboxed command failed (exit %d): %v", code, err)
	}
	fmt.Print(out)
}

func execpolicyMain(root rootArgs, args []string) {
	fs := flag.NewFlagSet("execpolicy", flag.ExitOnError)
	var sandboxMode string
	var approvalPolicy string
	fs.StringVar(&sandboxMode, "sandbox", "", "Sandbox mode to evaluate (read-only|workspace-write|danger-full-access)")
	fs.StringVar(&approvalPolicy, "approval", "", "Approval policy (never|on-request|untrusted|auto-deny)")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse execpolicy args: %v", err)
	}
	rt := applyRuntimeKVOverrides(defaultRuntimeConfig(), prependOverrides(root.overrides, nil))
	if sandboxMode == "" {
		sandboxMode = rt.SandboxMode
	}
	if approvalPolicy == "" {
		approvalPolicy = rt.ApprovalPolicy
	}
	pol := policy.Policy{SandboxMode: sandboxMode, ApprovalPolicy: approvalPolicy}
	cmd := pol.AllowCommand()
	write := pol.AllowWrite()
	fmt.Printf("command allowed=%t approval_required=%t reason=%s\n", cmd.Allowed, cmd.RequiresApproval, cmd.Reason)
	fmt.Printf("file_change allowed=%t approval_required=%t reason=%s\n", write.Allowed, write.RequiresApproval, write.Reason)
}

func mcpMain(root rootArgs, args []string) {
	if err := delegateEchoRS("mcp", root, args); err != nil {
		log.Fatalf("mcp command is not available: %v", err)
	}
}

func mcpServerMain(root rootArgs, args []string) {
	if err := delegateEchoRS("mcp-server", root, args); err != nil {
		log.Fatalf("mcp-server command is not available: %v", err)
	}
}

func cloudMain(root rootArgs, args []string) {
	if err := delegateEchoRS("cloud", root, args); err != nil {
		log.Fatalf("cloud tasks are not available: %v", err)
	}
}

func responsesProxyMain(root rootArgs, args []string) {
	if err := delegateEchoRS("responses-proxy", root, args); err != nil {
		log.Fatalf("responses proxy is not available: %v", err)
	}
}

func stdioToUDSMain(root rootArgs, args []string) {
	if err := delegateEchoRS("stdio-to-uds", root, args); err != nil {
		log.Fatalf("stdio-to-uds relay is not available: %v", err)
	}
}

func featuresMain(root rootArgs, args []string) {
	var overrides stringSlice
	fs := flag.NewFlagSet("features", flag.ExitOnError)
	fs.Var(&overrides, "c", "Override config value key=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse features args: %v", err)
	}
	allOverrides := prependOverrides(root.overrides, []string(overrides))
	for _, spec := range features.Specs {
		enabled := featureEnabled(spec.Key, allOverrides)
		fmt.Fprintf(os.Stdout, "%s\t%s\t%t\n", spec.Key, spec.Stage, enabled)
	}
}

func featureEnabled(key string, overrides []string) bool {
	enabled := features.DefaultEnabled(key)
	for _, raw := range overrides {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if !strings.HasPrefix(k, "features.") {
			continue
		}
		name := strings.TrimPrefix(k, "features.")
		if !strings.EqualFold(name, key) {
			continue
		}
		switch strings.ToLower(v) {
		case "true", "1", "t", "yes", "y", "on":
			enabled = true
		case "false", "0", "f", "no", "n", "off":
			enabled = false
		}
	}
	return enabled
}

func readTokenFromStdin() string {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("failed to read token from stdin: %v", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		log.Fatalf("no token provided on stdin")
	}
	return key
}

func promptToken() string {
	fmt.Print("Enter token: ")
	reader := bufio.NewReader(os.Stdin)
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)
	if key == "" {
		log.Fatalf("empty token provided")
	}
	return key
}

func delegateEchoRS(subcommand string, root rootArgs, args []string) error {
	bin := findEchoBinary()
	if bin == "" {
		return fmt.Errorf("echo-cli binary not found; cannot run %s", subcommand)
	}
	var prefix []string
	for _, ov := range root.overrides {
		prefix = append(prefix, "-c", ov)
	}
	cmd := exec.Command(bin, append(prefix, append([]string{subcommand}, args...)...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func findEchoBinary() string {
	candidates := []string{
		filepath.Join("bin", "echo-cli"),
		filepath.Join("bin", "echo"),
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(
			candidates,
			filepath.Join(dir, "bin", "echo-cli"),
			filepath.Join(dir, "..", "bin", "echo-cli"),
			filepath.Join(dir, "bin", "echo"),
			filepath.Join(dir, "..", "bin", "echo"),
		)
	}
	for _, cand := range candidates {
		if stat, err := os.Stat(cand); err == nil && !stat.IsDir() {
			return cand
		}
	}
	return ""
}
