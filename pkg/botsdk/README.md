# Nonoka External Bot SDK (Go)

Reference implementation of the [External Bot Protocol v1](../../docs/bot-protocol.md).
It provides:

- typed HTTP client for sending text, resolving receipts and reading history;
- Standard Webhooks signature verification with an optional deduper;
- a streaming reply helper (`NewStream` → `Write` → `Finalize`);
- typed `APIError` and retry handling for retryable failures.

No Python or Node SDK is published. Those languages use the HTTP contract in
`docs/bot-protocol.md`.

## Quick start

```go
client, err := botsdk.New(botsdk.Config{
    BaseURL: "http://localhost:8000",
    Token:   os.Getenv("NONOKA_BOT_TOKEN"),
    ReceiptTimeout: 3 * time.Second,
})
if err != nil {
    log.Fatal(err)
}

webhook, err := botsdk.NewWebhook(botsdk.WebhookConfig{
    Secret:  os.Getenv("NONOKA_WEBHOOK_SECRET"),
    Deduper: botsdk.NewMemoryDeduper(24*time.Hour, 10000),
})
if err != nil {
    log.Fatal(err)
}

handle := func(ctx context.Context, ev *botsdk.Event) error {
    if ev.Data.Message == nil {
        return nil
    }
    _, err := client.SendText(ctx, ev.Data.Message.Topic, "收到："+ev.Data.Message.Content)
    return err
}
webhook.OnMention(handle).OnDirect(handle)

http.Handle("/webhook", webhook)
log.Fatal(http.ListenAndServe(":8095", nil))
```

The full runnable example is in [`examples/bot`](../../examples/bot/main.go).
