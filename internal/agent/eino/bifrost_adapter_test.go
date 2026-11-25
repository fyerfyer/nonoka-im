package eino

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"nonoka-im/internal/agent/domain"
)

func TestBifrostTransportErrorSurvivesOpenAIAdapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"type":"budget_exceeded","error":{"message":"budget exceeded"}}`))
	}))
	defer srv.Close()

	model, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey: "placeholder", BaseURL: srv.URL, Model: "test",
		HTTPClient: &http.Client{Transport: &bifrostTransport{base: http.DefaultTransport, vk: "sk-bf-test"}},
	})
	if err != nil {
		t.Fatalf("new model: %v", err)
	}
	_, err = model.Generate(context.Background(), nil)
	var budgetErr *domain.BudgetError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected budget error through adapter, got %T %v", err, err)
	}
}
