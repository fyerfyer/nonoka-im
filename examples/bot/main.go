// Command bot is the reference Nonoka external bot.
//
// It receives mention/DM webhooks and replies with a short echo. Run it with:
//
//	NONOKA_BASE_URL=http://localhost:8000 \
//	NONOKA_BOT_TOKEN=nb_... \
//	NONOKA_WEBHOOK_SECRET=whsec_... \
//	go run ./examples/bot
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nonoka-im/pkg/botsdk"
)

func main() {
	baseURL := env("NONOKA_BASE_URL", "http://127.0.0.1:8000")
	token := os.Getenv("NONOKA_BOT_TOKEN")
	secret := os.Getenv("NONOKA_WEBHOOK_SECRET")
	if token == "" || secret == "" {
		log.Fatal("NONOKA_BOT_TOKEN and NONOKA_WEBHOOK_SECRET are required")
	}
	client, err := botsdk.New(botsdk.Config{
		BaseURL: baseURL, Token: token, ReceiptTimeout: 3 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}
	webhook, err := botsdk.NewWebhook(botsdk.WebhookConfig{
		Secret: secret, Deduper: botsdk.NewMemoryDeduper(24*time.Hour, 10000),
	})
	if err != nil {
		log.Fatal(err)
	}
	reply := func(ctx context.Context, event *botsdk.Event) error {
		if event.Data.Message == nil {
			return nil
		}
		text := strings.TrimSpace(event.Data.Message.Content)
		if text == "" {
			return nil
		}
		_, err := client.SendText(ctx, event.Data.Message.Topic,
			fmt.Sprintf("收到：%s", text),
			botsdk.WithClientMsgID("reply-"+event.ID),
		)
		return err
	}
	webhook.OnMention(reply).OnDirect(reply)

	mux := http.NewServeMux()
	mux.Handle("/webhook", webhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	addr := env("BOT_ADDR", ":8095")
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("example bot listening on %s/webhook", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
