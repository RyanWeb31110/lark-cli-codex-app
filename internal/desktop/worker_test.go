package desktop

import (
	"testing"
	"time"
)

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
