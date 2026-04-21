package desktop

import (
	"context"
	"fmt"
	"log"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	urlPattern         = regexp.MustCompile(`https?://[^\s]+|[A-Za-z0-9.-]+\.[A-Za-z]{2,}(?:/[^\s]*)?`)
	powerPattern       = regexp.MustCompile(`(\d+)\s*的\s*(\d+)\s*次方`)
	numberTokenPattern = regexp.MustCompile(`\d+(?:\.\d+)?|[()+\-*/^]`)
)

type WorkerConfig struct {
	PollInterval time.Duration
}

type Worker struct {
	queue  *Queue
	logger *log.Logger
	cfg    WorkerConfig
}

func NewWorker(queue *Queue, logger *log.Logger, cfg WorkerConfig) *Worker {
	if queue == nil {
		queue = DefaultQueue()
	}
	if logger == nil {
		logger = log.Default()
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	return &Worker{
		queue:  queue,
		logger: logger,
		cfg:    cfg,
	}
}

func (w *Worker) Serve(ctx context.Context) error {
	if err := w.processOnce(); err != nil {
		w.logger.Printf("desktop worker process error: %v", err)
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.processOnce(); err != nil {
				w.logger.Printf("desktop worker process error: %v", err)
			}
		}
	}
}

func (w *Worker) processOnce() error {
	task, err := w.queue.PopPending()
	if err != nil {
		return err
	}
	if task == nil {
		return nil
	}

	result, err := executeDesktopTask(task.RequestText)
	if err != nil {
		_, finishErr := w.queue.Fail(task.ID, err.Error(), true)
		if finishErr != nil {
			return fmt.Errorf("desktop task %s failed: %v (reply error: %w)", task.ID, err, finishErr)
		}
		return nil
	}

	_, err = w.queue.Complete(task.ID, result, true)
	return err
}

func executeDesktopTask(request string) (string, error) {
	text := strings.TrimSpace(request)
	if text == "" {
		return "", fmt.Errorf("桌面任务为空")
	}

	if result, handled, err := tryCalculatorTask(text); handled {
		return result, err
	}
	if result, handled, err := tryOpenURLTask(text); handled {
		return result, err
	}
	if result, handled, err := tryOpenAppTask(text); handled {
		return result, err
	}
	if result, handled, err := tryQuitAppTask(text); handled {
		return result, err
	}

	return "", fmt.Errorf("暂时只支持打开/关闭应用、打开链接、以及计算器计算这几类桌面任务")
}

func tryCalculatorTask(text string) (string, bool, error) {
	if !containsAny(strings.ToLower(text), []string{"计算器", "calculator"}) {
		return "", false, nil
	}
	if err := openApp("Calculator"); err != nil {
		return "", true, err
	}

	expr, displayExpr, err := extractExpression(text)
	if err != nil {
		return "已打开计算器。", true, nil
	}

	typedExpr, err := expandForCalculator(expr)
	if err != nil {
		return "", true, err
	}
	value, err := evalExpression(expr)
	if err != nil {
		return "", true, err
	}
	if err := typeIntoCalculator(typedExpr + "="); err != nil {
		if isAccessibilityDenied(err) {
			return fmt.Sprintf("已打开计算器。当前系统还没给辅助功能权限，所以我先直接算出 %s = %s。", displayExpr, formatNumber(value)), true, nil
		}
		return "", true, err
	}
	return fmt.Sprintf("已打开计算器，%s = %s。", displayExpr, formatNumber(value)), true, nil
}

func tryOpenURLTask(text string) (string, bool, error) {
	url := extractURL(text)
	if url == "" {
		return "", false, nil
	}

	app := ""
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "safari"):
		app = "Safari"
	case strings.Contains(lower, "chrome"):
		app = "Google Chrome"
	}

	var err error
	if app != "" {
		err = runCommand("open", "-a", app, url)
	} else {
		err = runCommand("open", url)
	}
	if err != nil {
		return "", true, err
	}

	if app != "" {
		return fmt.Sprintf("已打开 %s 并访问 %s。", app, url), true, nil
	}
	return fmt.Sprintf("已打开链接 %s。", url), true, nil
}

func tryOpenAppTask(text string) (string, bool, error) {
	app := detectOpenApp(text)
	if app == "" {
		return "", false, nil
	}
	if err := openApp(app); err != nil {
		return "", true, err
	}
	return fmt.Sprintf("已在这台 Mac 上打开%s。", app), true, nil
}

func tryQuitAppTask(text string) (string, bool, error) {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"关闭", "退出", "quit", "close"}) {
		return "", false, nil
	}

	app := detectKnownApp(text)
	if app == "" {
		return "", false, nil
	}
	script := fmt.Sprintf(`tell application %q to quit`, app)
	if err := runCommand("osascript", "-e", script); err != nil {
		return "", true, err
	}
	return fmt.Sprintf("已关闭%s。", app), true, nil
}

func openApp(app string) error {
	return runCommand("open", "-a", app)
}

func typeIntoCalculator(sequence string) error {
	if err := runCommand("osascript",
		"-e", `tell application "Calculator" to activate`,
		"-e", `delay 0.3`,
		"-e", `tell application "System Events" to keystroke "c" using command down`,
		"-e", `delay 0.1`,
		"-e", fmt.Sprintf(`tell application "System Events" to keystroke %q`, sequence),
	); err != nil {
		return fmt.Errorf("向计算器输入表达式失败: %w", err)
	}
	return nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func isAccessibilityDenied(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "not allowed to send keystrokes") ||
		strings.Contains(lower, "辅助功能") ||
		strings.Contains(lower, "1002")
}

func detectOpenApp(text string) string {
	lower := strings.ToLower(text)
	if !containsAny(lower, []string{"打开", "启动", "open ", "launch "}) {
		return ""
	}
	return detectKnownApp(text)
}

func detectKnownApp(text string) string {
	lower := strings.ToLower(text)
	for _, candidate := range []struct {
		match string
		app   string
	}{
		{"计算器", "Calculator"},
		{"calculator", "Calculator"},
		{"safari", "Safari"},
		{"chrome", "Google Chrome"},
		{"google chrome", "Google Chrome"},
		{"finder", "Finder"},
		{"访达", "Finder"},
		{"terminal", "Terminal"},
		{"终端", "Terminal"},
		{"system settings", "System Settings"},
		{"settings", "System Settings"},
		{"系统设置", "System Settings"},
		{"feishu", "Feishu"},
		{"飞书", "Feishu"},
		{"wechat", "WeChat"},
		{"微信", "WeChat"},
	} {
		if strings.Contains(lower, candidate.match) {
			return candidate.app
		}
	}
	return ""
}

func extractURL(text string) string {
	match := urlPattern.FindString(strings.TrimSpace(text))
	if match == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(match), "http://") || strings.HasPrefix(strings.ToLower(match), "https://") {
		return match
	}
	return "https://" + match
}

func extractExpression(text string) (string, string, error) {
	lower := normalizeMathText(text)
	lower = powerPattern.ReplaceAllString(lower, `$1^$2`)
	replacer := strings.NewReplacer(
		"计算一下", "",
		"算一下", "",
		"等于多少", "",
		"是多少", "",
		"结果", "",
		"？", "",
		"?", "",
		"，", "",
		",", "",
		"打开计算器", "",
		"calculator", "",
		"计算器", "",
		"然后", "",
		"并且", "",
		"再", "",
		"加上", "+",
		"加", "+",
		"减去", "-",
		"减", "-",
		"乘以", "*",
		"乘", "*",
		"x", "*",
		"除以", "/",
		"除", "/",
		"平方", "^2",
		"立方", "^3",
		"的二次方", "^2",
		"的三次方", "^3",
		" ", "",
	)
	expr := replacer.Replace(lower)
	tokens := numberTokenPattern.FindAllString(expr, -1)
	if len(tokens) == 0 {
		return "", "", fmt.Errorf("没有识别出可计算的表达式")
	}
	joined := strings.Join(tokens, "")
	display := strings.ReplaceAll(joined, "*", "×")
	return joined, display, nil
}

func normalizeMathText(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	return lower
}

func expandForCalculator(expr string) (string, error) {
	tokens := tokenize(expr)
	if len(tokens) == 0 {
		return "", fmt.Errorf("无法生成计算器表达式")
	}

	out := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if i+2 < len(tokens) && tokens[i+1] == "^" {
			base, err := strconv.ParseFloat(tokens[i], 64)
			if err != nil {
				return "", fmt.Errorf("暂不支持这个幂运算底数")
			}
			exp, err := strconv.Atoi(tokens[i+2])
			if err != nil || exp < 0 || exp > 9 || math.Trunc(base) != base {
				return "", fmt.Errorf("暂不支持这个幂运算写入计算器")
			}
			repeated := make([]string, exp)
			for j := 0; j < exp; j++ {
				repeated[j] = formatNumber(base)
			}
			out = append(out, "("+strings.Join(repeated, "*")+")")
			i += 2
			continue
		}
		out = append(out, tokens[i])
	}
	return strings.Join(out, ""), nil
}

func evalExpression(expr string) (float64, error) {
	parser := &mathParser{tokens: tokenize(expr)}
	if len(parser.tokens) == 0 {
		return 0, fmt.Errorf("空表达式")
	}
	value, err := parser.parseExpression()
	if err != nil {
		return 0, err
	}
	if parser.pos != len(parser.tokens) {
		return 0, fmt.Errorf("表达式包含无法解析的内容")
	}
	return value, nil
}

func tokenize(expr string) []string {
	return numberTokenPattern.FindAllString(expr, -1)
}

func formatNumber(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return strconv.FormatInt(int64(math.Round(value)), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

type mathParser struct {
	tokens []string
	pos    int
}

func (p *mathParser) parseExpression() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.hasNext() {
		op := p.peek()
		if op != "+" && op != "-" {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == "+" {
			left += right
		} else {
			left -= right
		}
	}
	return left, nil
}

func (p *mathParser) parseTerm() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}
	for p.hasNext() {
		op := p.peek()
		if op != "*" && op != "/" {
			break
		}
		p.pos++
		right, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		if op == "*" {
			left *= right
		} else {
			left /= right
		}
	}
	return left, nil
}

func (p *mathParser) parsePower() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.hasNext() && p.peek() == "^" {
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		left = math.Pow(left, right)
	}
	return left, nil
}

func (p *mathParser) parseFactor() (float64, error) {
	if !p.hasNext() {
		return 0, fmt.Errorf("表达式不完整")
	}

	token := p.peek()
	p.pos++

	switch token {
	case "(":
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if !p.hasNext() || p.peek() != ")" {
			return 0, fmt.Errorf("缺少右括号")
		}
		p.pos++
		return value, nil
	case "-":
		value, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -value, nil
	default:
		value, err := strconv.ParseFloat(token, 64)
		if err != nil {
			return 0, fmt.Errorf("无法解析数字 %q", token)
		}
		return value, nil
	}
}

func (p *mathParser) hasNext() bool {
	return p.pos < len(p.tokens)
}

func (p *mathParser) peek() string {
	return p.tokens[p.pos]
}
