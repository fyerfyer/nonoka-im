package app

import "strings"

// PublicFailureMessage maps an internal failure to a user-safe message. Raw
// provider/tool errors stay in the run row and logs for operators; chat and
// API consumers only see a stable description.
func PublicFailureMessage(code, raw string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	switch code {
	case "quota_exceeded", "budget_exceeded", "rate_limited":
		// These messages are constructed by the quota/transport layers and are
		// intended to be shown to the user (limits, retry hints).
		if msg := strings.TrimSpace(raw); msg != "" {
			return msg
		}
		return "The agent run was blocked by its usage policy."
	case "canceled", "cancelled":
		return "The agent run was canceled."
	case "timeout":
		return "The agent run timed out. Please try again."
	case "access_denied":
		return "You do not have access to complete this action."
	case "invalid_config":
		return "The agent configuration is invalid. Contact the agent owner."
	case "infra_unavailable":
		return "The agent service is temporarily unavailable. Please retry shortly."
	case "tool_error":
		return "A tool used by the agent failed."
	case "model_error":
		return "The model provider returned an error. Please retry."
	case "manual_retry":
		return "This run may have already performed an action. Review the result before retrying."
	default:
		return "The agent run failed. Please try again."
	}
}
