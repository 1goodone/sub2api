package service

import (
	"fmt"
	"strconv"
	"strings"
)

// LocalProtocolDiagnostic is derived from a bounded, validated worker error.
// It contains no provider text and does not replace the task's ownership or state.
type LocalProtocolDiagnostic struct {
	Stage     string `json:"stage"`
	Reason    string `json:"reason"`
	ElapsedMS int    `json:"elapsed_ms"`
	ExitCode  int    `json:"exit_code"`
}

func localProtocolError(raw string) (string, *LocalProtocolDiagnostic, bool) {
	msg := strings.TrimPrefix(strings.TrimSpace(raw), "WorkerError: ")
	if !strings.HasPrefix(msg, "local_protocol_failed") {
		return "", nil, false
	}
	invalid := "local protocol failed; invalid diagnostic omitted"
	if len(msg) > 256 {
		return invalid, nil, true
	}
	parts := strings.Split(msg, " ")
	if len(parts) != 5 || parts[0] != "local_protocol_failed" {
		return invalid, nil, true
	}
	stage, okStage := strings.CutPrefix(parts[1], "stage=")
	reason, okReason := strings.CutPrefix(parts[2], "reason=")
	elapsed, okElapsed := strings.CutPrefix(parts[3], "elapsed_ms=")
	exit, okExit := strings.CutPrefix(parts[4], "exit=")
	elapsedMS, elapsedErr := strconv.Atoi(elapsed)
	exitCode, exitErr := strconv.Atoi(exit)
	if !okStage || !okReason || !okElapsed || !okExit || elapsedErr != nil || exitErr != nil ||
		elapsedMS < 0 || elapsedMS > 3600000 || exitCode < -255 || exitCode > 255 ||
		!validLocalProtocolStage(stage) || !validLocalProtocolReason(reason) {
		return invalid, nil, true
	}
	diagnostic := &LocalProtocolDiagnostic{Stage: stage, Reason: reason, ElapsedMS: elapsedMS, ExitCode: exitCode}
	return fmt.Sprintf("local_protocol_failed stage=%s reason=%s elapsed_ms=%d exit=%d", stage, reason, elapsedMS, exitCode), diagnostic, true
}

func validLocalProtocolStage(stage string) bool {
	switch stage {
	case "starting", "web_login", "password_accepted", "mfa_challenge", "mfa_accepted", "web_redirect",
		"oauth_authorize", "session_select", "workspace_select", "callback_validation", "token_exchange", "oauth_output", "completed":
		return true
	}
	return false
}

func validLocalProtocolReason(reason string) bool {
	switch reason {
	case "protocol_error", "password_rejected", "totp_rejected", "interaction_required", "security_check",
		"session_unavailable", "workspace_mismatch", "callback_missing", "callback_invalid", "callback_state_mismatch",
		"pkce_invalid", "token_exchange_failed", "credentials_incomplete", "transport_error", "runtime_error",
		"process_failed", "timeout", "diagnostic_invalid", "output_invalid":
		return true
	}
	return false
}
