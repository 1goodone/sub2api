// Plant oai-did on the same proxy before an OAuth authorize request.
// One attempt only: the worker owns the proxy and must not rotate it.

export async function warmChatGPTProtocol(client) {
  const url = 'https://chatgpt.com/api/auth/csrf';
  try {
    await client.follow(url);
  } catch (error) {
    const message = String(error?.message || error || '');
    if (/security-check|PROXY_RISK_CONTROL|403/.test(message)) throw new Error('PROTOCOL_WARMUP_FAILED');
    throw error;
  }
  const deviceId = client.jar?.value?.('oai-did', 'https://chatgpt.com')
    || client.jar?.value?.('oai-did', 'https://auth.openai.com');
  if (!deviceId) throw new Error('PROTOCOL_WARMUP_FAILED');
  return deviceId;
}
