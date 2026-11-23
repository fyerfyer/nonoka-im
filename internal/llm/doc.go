// Package llm owns the platform LLM service catalog and the Bifrost
// control-plane client. Providers, provider credentials, routing, budgets and
// rate limits live in the gateway; this package only maps a platform service to
// one gateway virtual key and resolves the endpoint used by an agent run.
package llm
