package desktop

import (
	"testing"
	"time"
)

const sampleTrendingHTML = `
<article class="Box-row">
  <h2 class="h3 lh-condensed">
    <a href="/openai/openai-agents-python" class="Link">
      <span class="text-normal">openai /</span>
      openai-agents-python
    </a>
  </h2>
  <p class="col-9 color-fg-muted my-1 tmp-pr-4">
    Lightweight, powerful agents.
  </p>
  <div class="f6 color-fg-muted mt-2">
    <span itemprop="programmingLanguage">Python</span>
    <span>1,234 stars today</span>
  </div>
</article>
`

func TestExtractExpression(t *testing.T) {
	expr, display, err := extractExpression("打开计算器，计算一下 2 的三次方加 5 是多少？")
	if err != nil {
		t.Fatalf("extract expression: %v", err)
	}
	if expr != "2^3+5" {
		t.Fatalf("unexpected expr: %s", expr)
	}
	if display != "2^3+5" {
		t.Fatalf("unexpected display expr: %s", display)
	}
}

func TestEvalExpression(t *testing.T) {
	value, err := evalExpression("2^3+5")
	if err != nil {
		t.Fatalf("eval expression: %v", err)
	}
	if value != 13 {
		t.Fatalf("unexpected value: %v", value)
	}
}

func TestExpandForCalculator(t *testing.T) {
	typed, err := expandForCalculator("2^3+5")
	if err != nil {
		t.Fatalf("expand for calculator: %v", err)
	}
	if typed != "(2*2*2)+5" {
		t.Fatalf("unexpected typed expr: %s", typed)
	}
}

func TestWorkerDefaults(t *testing.T) {
	worker := NewWorker(nil, nil, WorkerConfig{})
	if worker.cfg.PollInterval != 2*time.Second {
		t.Fatalf("unexpected poll interval: %s", worker.cfg.PollInterval)
	}
}

func TestParseGitHubTrending(t *testing.T) {
	repos := parseGitHubTrending(sampleTrendingHTML)
	if len(repos) != 1 {
		t.Fatalf("unexpected repo count: %d", len(repos))
	}
	repo := repos[0]
	if repo.Name != "openai/openai-agents-python" {
		t.Fatalf("unexpected repo name: %s", repo.Name)
	}
	if repo.Description != "Lightweight, powerful agents." {
		t.Fatalf("unexpected description: %s", repo.Description)
	}
	if repo.Language != "Python" {
		t.Fatalf("unexpected language: %s", repo.Language)
	}
	if repo.StarsToday != "1,234" {
		t.Fatalf("unexpected stars today: %s", repo.StarsToday)
	}
}
