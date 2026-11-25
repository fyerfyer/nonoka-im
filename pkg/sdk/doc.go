// Package sdk is the Nonoka IM + Agent platform Go SDK.
//
// The package intentionally stays flat: resources (auth.go, message.go,
// agent.go, llm.go, ...) are files in one package so they can share the
// transport, error, pagination, token and event contracts without exposing an
// internal transport package or forcing callers to import several type
// packages. Go's import cycle rules make per-resource subpackages a poor fit
// for a client this size.
//
// The main entry point is Client. Resources are grouped by domain:
//
//	Auth, Users, Conversations, Messages, Groups, Files,
//	Agents, Models, Quotas, Audit, MCP, Skills, Runs, Realtime
//
// Public types never expose generated protobuf types. HTTP writes use stable
// client message IDs; realtime events and RunStream are typed and ordered.
package sdk
