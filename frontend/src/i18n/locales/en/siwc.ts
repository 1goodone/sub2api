export default {
  "protocol": "OpenAI request protocol",
  "codex": "Codex (keep existing BPS / Prism settings)",
  "siwc": "SIWC API (Sign in with ChatGPT)",
  "protocolHint": "Switch after authorization and save the account to apply. SIWC uses your ChatGPT plan through the official Responses API. Codex credentials are retained separately.",
  "bound": "SIWC bound (credentials present; availability not verified)",
  "unbound": "SIWC not bound",
  "automatic": "Authorize with saved password / 2FA",
  "browser": "Browser authorization",
  "refresh": "Refresh status",
  "loginHint": "Automatic authorization reuses the password and TOTP saved in Credential Guard. Use browser authorization for initial consent or additional verification. Tokens refresh automatically afterward.",
  "running": "SIWC authorization in progress…",
  "continue": "Continue with ChatGPT",
  "callbackHint": "After authorization, copy the full loopback callback URL from the browser address bar, even if the local page cannot be reached. Paste it below.",
  "callback": "Full callback URL",
  "complete": "Complete binding",
  "failed": "SIWC operation failed. Refresh and try again."
}
