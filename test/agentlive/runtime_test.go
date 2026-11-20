package agentlive

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nonoka-im/internal/agent/eino"
	"nonoka-im/internal/agent/port"
)

func liveConfig(t *testing.T) (string, string, string) {
	t.Helper()
	if os.Getenv("NONOKA_AGENT_LIVE") != "1" {
		t.Skip("live suite disabled; set NONOKA_AGENT_LIVE=1")
	}
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		file, err := os.Open(filepath.Join("..", "..", ".env"))
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if !strings.HasPrefix(line, "DEEPSEEK_API_KEY=") {
					continue
				}
				key = strings.Trim(strings.TrimPrefix(line, "DEEPSEEK_API_KEY="), `"'`)
				break
			}
		}
	}
	if key == "" {
		t.Skip("DEEPSEEK_API_KEY absent")
	}
	base := os.Getenv("DEEPSEEK_BASE_URL")
	if base == "" {
		base = "https://api.deepseek.com/v1"
	}
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = "deepseek-chat"
	}
	return key, base, model
}

func TestDeepSeekTwoTurnsAndSummaryAcrossRuntime(t *testing.T) {
	key, base, model := liveConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	firstRuntime, err := eino.NewRuntime(ctx, key, base, model)
	if err != nil {
		t.Fatal(err)
	}
	instruction := "Answer in one short sentence. When asked for the codeword, repeat it exactly."
	first, err := firstRuntime.Run(ctx, port.Input{Instruction: instruction, Text: "The codeword for this chat is marigold. Acknowledge it briefly."})
	if err != nil || strings.TrimSpace(first.Text) == "" {
		t.Fatalf("first DeepSeek turn: text empty=%t err=%v", first.Text == "", err)
	}
	secondRuntime, err := eino.NewRuntime(ctx, key, base, model)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondRuntime.Run(ctx, port.Input{Instruction: instruction, History: []port.Turn{{ID: 1, Input: "The codeword for this chat is marigold. Acknowledge it briefly.", Output: first.Text}}, Text: "What is this chat's codeword?"})
	if err != nil || !strings.Contains(strings.ToLower(second.Text), "marigold") {
		t.Fatalf("second DeepSeek turn did not cite history: text=%q err=%v", second.Text, err)
	}
	withSummary, err := secondRuntime.Run(ctx, port.Input{Instruction: instruction, Text: "What is the codeword?", History: []port.Turn{{ID: 1, Input: "The codeword for this chat is marigold.", Output: first.Text}}, NeedsSummary: true, TriggerTokens: 1, SummaryThroughTurnID: 1})
	if err != nil || strings.TrimSpace(withSummary.Summary) == "" || withSummary.SummaryThroughTurnID != 1 {
		t.Fatalf("DeepSeek summary unavailable: summary empty=%t err=%v", withSummary.Summary == "", err)
	}
	restartedRuntime, err := eino.NewRuntime(ctx, key, base, model)
	if err != nil {
		t.Fatal(err)
	}
	last, err := restartedRuntime.Run(ctx, port.Input{Instruction: instruction, Summary: withSummary.Summary, Text: "What is this chat's codeword?"})
	if err != nil || !strings.Contains(strings.ToLower(last.Text), "marigold") {
		t.Fatalf("summary not usable after runtime restart: text=%q err=%v", last.Text, err)
	}
	t.Logf("DeepSeek runtime acceptance passed: model=%s, turns=4", model)
}
