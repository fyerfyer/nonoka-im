// Package llm owns the Bifrost control-plane client, the flat local model
// catalog and the lifecycle of per-agent/playground virtual keys. Provider
// credentials, routing, pricing and hard budgets live in Bifrost; this package
// only projects the catalog and resolves a run's inference endpoint.
package llm
