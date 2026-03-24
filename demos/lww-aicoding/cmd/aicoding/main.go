package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aicoding/internal/agent"
	"aicoding/internal/config"
	"aicoding/internal/provider"
	"aicoding/internal/session"
	"aicoding/internal/tools"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiBlue  = "\x1b[94m"
	ansiGray  = "\x1b[90m"
)

type slashCommand struct {
	Name        string
	Usage       string
	Description string
}

var slashCommands = []slashCommand{
	{Name: "/help", Usage: "/help", Description: "Show available commands"},
	{Name: "/session", Usage: "/session", Description: "Show the current session"},
	{Name: "/sessions", Usage: "/sessions [limit]", Description: "List recent sessions"},
	{Name: "/resume", Usage: "/resume <id>", Description: "Resume a recent session by id or prefix"},
	{Name: "/new", Usage: "/new", Description: "Start a new session in the current workspace"},
	{Name: "/plan", Usage: "/plan [create|add|start|done|pending|clear]", Description: "Show or manage the task plan"},
	{Name: "/exit", Usage: "/exit", Description: "Exit the CLI"},
	{Name: "/quit", Usage: "/quit", Description: "Exit the CLI"},
}

func main() {
	configureConsoleEncoding()
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runChat(nil, stdin, stdout, stderr)
	}

	switch args[0] {
	case "chat":
		return runChat(args[1:], stdin, stdout, stderr)
	case "run":
		return runOneShot(args[1:], stdin, stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return runChat(args, stdin, stdout, stderr)
	}
}

func runChat(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Resume an existing session")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(args); err != nil {
		return err
	}

	app, store, sess, err := bootstrap(*configPath, *model, *sessionID, *streamOverride, *workspaceOverride, *maxIterations, stdin, stdout)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "%ssession%s %s\n", ansiDim, ansiReset, sess.ID)
	fmt.Fprintf(stdout, "%sworkspace%s %s\n", ansiDim, ansiReset, sess.Workspace)
	fmt.Fprintln(stdout, "Type /help for commands, /exit to quit.")

	scanner := bufio.NewScanner(stdin)
	for {
		fmt.Fprint(stdout, promptPrefix())
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			fmt.Fprintln(stdout)
			return nil
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			completed, suggestions := completeSlashCommand(input)
			switch {
			case len(suggestions) > 1:
				printCommandSuggestions(stdout, input, suggestions)
				continue
			case completed != input:
				fmt.Fprintf(stdout, "%scommand%s %s\n", ansiDim, ansiReset, completed)
				input = completed
			}

			nextSess, shouldExit, handled, err := handleSlashCommand(stdout, store, sess, input)
			if err != nil {
				return err
			}
			if handled {
				sess = nextSess
				if shouldExit {
					return nil
				}
				continue
			}
		}

		if _, err := app.RunPrompt(context.Background(), sess, input, stdout); err != nil {
			return err
		}
	}
}

func runOneShot(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Reuse an existing session")
	prompt := fs.String("prompt", "", "Prompt to send")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*prompt) == "" {
		*prompt = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*prompt) == "" {
		return errors.New("run requires -prompt or trailing prompt text")
	}

	app, _, sess, err := bootstrap(*configPath, *model, *sessionID, *streamOverride, *workspaceOverride, *maxIterations, stdin, stdout)
	if err != nil {
		return err
	}

	_, err = app.RunPrompt(context.Background(), sess, *prompt, stdout)
	return err
}

func promptPrefix() string {
	return "\n" + ansiBlue + "aicoding>" + ansiReset + " "
}

func bootstrap(configPath, modelOverride, sessionID, streamOverride, workspaceOverride string, maxIterationsOverride int, stdin io.Reader, stdout io.Writer) (*agent.Runner, *session.Store, *session.Session, error) {
	workspace, err := resolveWorkspace(workspaceOverride)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err := config.Load(workspace, configPath)
	if err != nil {
		return nil, nil, nil, err
	}
	if modelOverride != "" {
		cfg.Provider.Model = modelOverride
	}
	if streamOverride != "" {
		parsed, err := strconv.ParseBool(streamOverride)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid -stream value: %w", err)
		}
		cfg.Stream = parsed
	}
	if maxIterationsOverride < 0 {
		return nil, nil, nil, errors.New("-max-iterations must be greater than 0")
	}
	if maxIterationsOverride > 0 {
		cfg.MaxIterations = maxIterationsOverride
	}

	apiKey := cfg.Provider.ResolveAPIKey()
	if apiKey == "" {
		return nil, nil, nil, errors.New("missing API key; set AICODING_API_KEY or configure provider.api_key")
	}

	store, err := session.NewStore(cfg.SessionDir)
	if err != nil {
		return nil, nil, nil, err
	}

	var sess *session.Session
	if sessionID != "" {
		sess, err = store.Load(sessionID)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		sess = session.New(workspace)
		if err := store.Save(sess); err != nil {
			return nil, nil, nil, err
		}
	}

	cfg.Provider.APIKey = apiKey
	client, err := provider.NewClient(cfg.Provider)
	if err != nil {
		return nil, nil, nil, err
	}

	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config:    cfg,
		Client:    client,
		Store:     store,
		Registry:  tools.DefaultRegistry(),
		Stdin:     stdin,
		Stdout:    stdout,
	})

	return runner, store, sess, nil
}

func handleSlashCommand(stdout io.Writer, store *session.Store, current *session.Session, input string) (*session.Session, bool, bool, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return current, false, false, nil
	}

	switch fields[0] {
	case "/exit", "/quit":
		return current, true, true, nil
	case "/help":
		printHelp(stdout)
		return current, false, true, nil
	case "/session":
		printCurrentSession(stdout, current)
		return current, false, true, nil
	case "/sessions":
		limit, err := parseOptionalLimit(fields)
		if err != nil {
			return current, false, true, err
		}
		if err := printSessions(stdout, store, current.ID, limit); err != nil {
			return current, false, true, err
		}
		return current, false, true, nil
	case "/resume":
		if len(fields) < 2 {
			fmt.Fprintln(stdout, "usage: /resume <id>")
			return current, false, true, nil
		}
		id, err := resolveSessionID(store, fields[1])
		if err != nil {
			return current, false, true, err
		}
		next, err := store.Load(id)
		if err != nil {
			return current, false, true, err
		}
		if !sameWorkspace(current.Workspace, next.Workspace) {
			return current, false, true, fmt.Errorf("session %s belongs to workspace %s, current workspace is %s", next.ID, next.Workspace, current.Workspace)
		}
		fmt.Fprintf(stdout, "%sresumed%s %s\n", ansiDim, ansiReset, next.ID)
		printCurrentSession(stdout, next)
		return next, false, true, nil
	case "/new":
		next := session.New(current.Workspace)
		if err := store.Save(next); err != nil {
			return current, false, true, err
		}
		fmt.Fprintf(stdout, "%snew session%s %s\n", ansiDim, ansiReset, next.ID)
		printCurrentSession(stdout, next)
		return next, false, true, nil
	case "/plan":
		return handlePlanCommand(stdout, store, current, input)
	default:
		fmt.Fprintf(stdout, "unknown command: %s\n", fields[0])
		printCommandSuggestions(stdout, fields[0], commandNames())
		return current, false, true, nil
	}
}

func handlePlanCommand(stdout io.Writer, store *session.Store, current *session.Session, input string) (*session.Session, bool, bool, error) {
	fields := strings.Fields(input)
	if len(fields) == 1 {
		printPlan(stdout, current)
		return current, false, true, nil
	}

	sub := strings.ToLower(fields[1])
	switch sub {
	case "create":
		raw := strings.TrimSpace(strings.TrimPrefix(input, "/plan create"))
		steps := parsePlanSteps(raw)
		if len(steps) == 0 {
			return current, false, true, errors.New("usage: /plan create step one | step two | step three")
		}
		if len(steps) == 1 {
			steps = generatePlanSteps(steps[0])
		}
		if err := replacePlan(store, current, steps); err != nil {
			return current, false, true, err
		}
		fmt.Fprintf(stdout, "%splan%s created with %d steps\n", ansiDim, ansiReset, len(current.Plan))
		printPlan(stdout, current)
		return current, false, true, nil
	case "add":
		step := strings.TrimSpace(strings.TrimPrefix(input, "/plan add"))
		if step == "" {
			return current, false, true, errors.New("usage: /plan add <step>")
		}
		current.Plan = append(current.Plan, session.PlanItem{Step: step, Status: defaultAddedPlanStatus(current.Plan)})
		if err := store.Save(current); err != nil {
			return current, false, true, err
		}
		fmt.Fprintf(stdout, "%splan%s added step %d\n", ansiDim, ansiReset, len(current.Plan))
		printPlan(stdout, current)
		return current, false, true, nil
	case "start":
		index, err := parsePlanIndex(fields)
		if err != nil {
			return current, false, true, err
		}
		if err := setPlanStatus(current, index, "in_progress"); err != nil {
			return current, false, true, err
		}
		if err := store.Save(current); err != nil {
			return current, false, true, err
		}
		printPlan(stdout, current)
		return current, false, true, nil
	case "done":
		index, err := parsePlanIndex(fields)
		if err != nil {
			return current, false, true, err
		}
		if err := setPlanStatus(current, index, "completed"); err != nil {
			return current, false, true, err
		}
		if err := store.Save(current); err != nil {
			return current, false, true, err
		}
		printPlan(stdout, current)
		return current, false, true, nil
	case "pending":
		index, err := parsePlanIndex(fields)
		if err != nil {
			return current, false, true, err
		}
		if err := setPlanStatus(current, index, "pending"); err != nil {
			return current, false, true, err
		}
		if err := store.Save(current); err != nil {
			return current, false, true, err
		}
		printPlan(stdout, current)
		return current, false, true, nil
	case "clear":
		current.Plan = current.Plan[:0]
		if err := store.Save(current); err != nil {
			return current, false, true, err
		}
		fmt.Fprintln(stdout, "Plan cleared.")
		return current, false, true, nil
	default:
		goal := strings.TrimSpace(strings.TrimPrefix(input, "/plan"))
		if goal == "" {
			printPlan(stdout, current)
			return current, false, true, nil
		}
		steps := generatePlanSteps(goal)
		if err := replacePlan(store, current, steps); err != nil {
			return current, false, true, err
		}
		fmt.Fprintf(stdout, "%splan%s created with %d steps\n", ansiDim, ansiReset, len(current.Plan))
		printPlan(stdout, current)
		return current, false, true, nil
	}
}

func replacePlan(store *session.Store, current *session.Session, steps []string) error {
	current.Plan = make([]session.PlanItem, 0, len(steps))
	for i, step := range steps {
		status := "pending"
		if i == 0 {
			status = "in_progress"
		}
		current.Plan = append(current.Plan, session.PlanItem{Step: step, Status: status})
	}
	return store.Save(current)
}

func parsePlanSteps(raw string) []string {
	parts := strings.Split(raw, "|")
	steps := make([]string, 0, len(parts))
	for _, part := range parts {
		step := strings.TrimSpace(part)
		if step != "" {
			steps = append(steps, step)
		}
	}
	return steps
}

func generatePlanSteps(goal string) []string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return []string{"明确目标并梳理约束", "实现核心改动", "验证结果并整理说明"}
	}

	lower := strings.ToLower(goal)
	switch {
	case containsAny(lower, "扫雷", "小游戏", "game", "游戏"):
		return []string{
			"设计页面结构与棋盘交互",
			"实现棋盘数据、雷区生成与初始化逻辑",
			"实现点击展开、插旗与胜负判断",
			"联调体验并验证功能完整性",
		}
	case containsAny(lower, "html", "页面", "前端", "ui", "界面", "react", "vue", "css", "javascript"):
		return []string{
			"梳理页面结构与交互需求",
			"实现界面骨架与基础样式",
			"实现核心交互与状态逻辑",
			"联调并验证页面表现",
		}
	case containsAny(lower, "api", "接口", "后端", "服务", "server", "数据库"):
		return []string{
			"梳理接口需求与数据流",
			"设计或调整数据结构与边界",
			"实现核心接口与业务逻辑",
			"补充验证并检查回归影响",
		}
	case containsAny(lower, "重构", "优化", "refactor", "cleanup"):
		return []string{
			"梳理现状与改造边界",
			"调整结构并实现核心重构",
			"清理兼容问题与冗余逻辑",
			"验证行为一致性并整理结果",
		}
	case containsAny(lower, "测试", "test", "修复", "fix", "bug"):
		return []string{
			"定位问题与影响范围",
			"实现修复或补充测试",
			"运行验证并确认回归情况",
		}
	default:
		return []string{
			"理解目标并梳理相关上下文",
			"设计实现方案与拆分步骤",
			"完成核心实现或改动",
			"验证结果并整理说明",
		}
	}
}

func containsAny(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func parsePlanIndex(fields []string) (int, error) {
	if len(fields) < 3 {
		return 0, errors.New("plan command requires a 1-based step index")
	}
	index, err := strconv.Atoi(fields[2])
	if err != nil || index <= 0 {
		return 0, errors.New("plan step index must be a positive integer")
	}
	return index - 1, nil
}

func defaultAddedPlanStatus(plan []session.PlanItem) string {
	for _, item := range plan {
		if item.Status == "in_progress" {
			return "pending"
		}
	}
	return "in_progress"
}

func setPlanStatus(current *session.Session, index int, status string) error {
	if len(current.Plan) == 0 {
		return errors.New("no active plan")
	}
	if index < 0 || index >= len(current.Plan) {
		return fmt.Errorf("plan step %d is out of range", index+1)
	}
	if status == "in_progress" {
		for i := range current.Plan {
			if i != index && current.Plan[i].Status == "in_progress" {
				current.Plan[i].Status = "pending"
			}
		}
	}
	current.Plan[index].Status = status
	return nil
}

func completeSlashCommand(input string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return input, nil
	}

	matches := matchSlashCommands(fields[0])
	if len(matches) == 0 {
		return input, nil
	}
	if len(matches) > 1 && !containsCommand(matches, fields[0]) {
		return input, matches
	}
	if containsCommand(matches, fields[0]) {
		return input, nil
	}

	completed := matches[0]
	if len(fields) == 1 {
		return completed, nil
	}
	return completed + " " + strings.Join(fields[1:], " "), nil
}

func matchSlashCommands(prefix string) []string {
	matches := make([]string, 0, len(slashCommands))
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd.Name)
		}
	}
	return matches
}

func containsCommand(commands []string, target string) bool {
	for _, cmd := range commands {
		if cmd == target {
			return true
		}
	}
	return false
}

func printHelp(w io.Writer) {
	for _, cmd := range slashCommands {
		fmt.Fprintf(w, "%-42s %s\n", cmd.Usage, cmd.Description)
	}
	fmt.Fprintln(w, "  /plan create step one | step two | step three")
	fmt.Fprintln(w, "  /plan <natural language goal>")
	fmt.Fprintln(w, "  /plan add <step>")
	fmt.Fprintln(w, "  /plan start <index>")
	fmt.Fprintln(w, "  /plan done <index>")
	fmt.Fprintln(w, "  /plan pending <index>")
	fmt.Fprintln(w, "  /plan clear")
}

func printCurrentSession(w io.Writer, sess *session.Session) {
	fmt.Fprintf(w, "%ssession%s %s\n", ansiDim, ansiReset, sess.ID)
	fmt.Fprintf(w, "%sworkspace%s %s\n", ansiDim, ansiReset, sess.Workspace)
	fmt.Fprintf(w, "%supdated%s %s\n", ansiDim, ansiReset, sess.UpdatedAt.Local().Format(time.DateTime))
}

func printSessions(w io.Writer, store *session.Store, currentID string, limit int) error {
	summaries, err := store.List(limit)
	if err != nil {
		return err
	}
	if len(summaries) == 0 {
		fmt.Fprintln(w, "No saved sessions.")
		return nil
	}

	fmt.Fprintf(w, "%srecent sessions%s\n", ansiBold, ansiReset)
	for _, item := range summaries {
		marker := " "
		if item.ID == currentID {
			marker = "*"
		}
		preview := item.LastUserMessage
		if preview == "" {
			preview = "(no user prompt yet)"
		}
		fmt.Fprintf(w, "%s %s  %s  %2d msgs  %s\n", marker, shortID(item.ID), item.UpdatedAt.Local().Format("2006-01-02 15:04"), item.MessageCount, preview)
		fmt.Fprintf(w, "%s    %s%s\n", ansiGray, item.Workspace, ansiReset)
	}
	return nil
}

func resolveSessionID(store *session.Store, prefix string) (string, error) {
	summaries, err := store.List(0)
	if err != nil {
		return "", err
	}

	matches := make([]string, 0, 4)
	for _, item := range summaries {
		if item.ID == prefix {
			return item.ID, nil
		}
		if strings.HasPrefix(item.ID, prefix) {
			matches = append(matches, item.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("session not found: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("session prefix %q matched multiple sessions", prefix)
	}
}

func parseOptionalLimit(fields []string) (int, error) {
	if len(fields) < 2 {
		return 8, nil
	}
	limit, err := strconv.Atoi(fields[1])
	if err != nil || limit <= 0 {
		return 0, errors.New("/sessions limit must be a positive integer")
	}
	return limit, nil
}

func sameWorkspace(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		left = a
	}
	right, err := filepath.Abs(b)
	if err != nil {
		right = b
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func printPlan(w io.Writer, sess *session.Session) {
	if len(sess.Plan) == 0 {
		fmt.Fprintln(w, "No active plan.")
		return
	}
	for i, item := range sess.Plan {
		fmt.Fprintf(w, "%d. [%s] %s\n", i+1, item.Status, item.Step)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "aicoding chat [-config path] [-model name] [-session id] [-stream true|false] [-workspace path] [-max-iterations n]")
	fmt.Fprintln(w, "aicoding run -prompt \"task\" [-config path] [-model name] [-session id] [-stream true|false] [-workspace path] [-max-iterations n]")
}

func printCommandSuggestions(w io.Writer, input string, suggestions []string) {
	fmt.Fprintf(w, "%smatches%s for %s:\n", ansiDim, ansiReset, input)
	for _, suggestion := range suggestions {
		fmt.Fprintf(w, "  %s\n", suggestion)
	}
}

func commandNames() []string {
	items := make([]string, 0, len(slashCommands))
	for _, cmd := range slashCommands {
		items = append(items, cmd.Name)
	}
	return items
}

func shortID(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:16]
}

func resolveWorkspace(workspaceOverride string) (string, error) {
	if strings.TrimSpace(workspaceOverride) == "" {
		return os.Getwd()
	}
	return filepath.Abs(workspaceOverride)
}
