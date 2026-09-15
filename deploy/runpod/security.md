# Inference endpoint security

The RunPod HTTP proxy provides TLS for the public URL. vLLM enforces the
`Authorization: Bearer` credential through its `--api-key` option.

Operational rules:

1. Generate a random token of at least 24 characters in a password manager.
2. Set it as `VLLM_API_KEY` in the RunPod console, never in a checked-in file.
3. Store the same value only as `STS_QWEN_API_KEY` in the gateway environment.
4. Do not put the token in the Apple `.xcconfig` or application bundle.
5. Rotate the token after a screen share, debug capture, or suspected leak.
6. Expose only port 8091 through the HTTPS proxy. Enable SSH only while
   diagnosing the Pod and remove it afterwards.

The gateway must redact authorization headers, audio, transcripts, and tool
arguments from normal logs.

