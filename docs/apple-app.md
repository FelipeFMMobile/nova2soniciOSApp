# App Apple

O app macOS/iOS continua conectado exclusivamente ao protocolo público do
gateway:

- URL local: `ws://127.0.0.1:8080/v1/voice`;
- token: `STS_DEVELOPMENT_TOKEN`;
- provider: Nova 2 Sonic.

O app não conhece LiteLLM, AWS ou chaves cloud. Ele captura PCM16 mono 16 kHz,
reproduz PCM16 24 kHz na ordem das sequências e limpa imediatamente o áudio
quando recebe interrupção.

Para iPhone físico, execute `make dev`, escolha rede local e use o IP do Mac.
Como `ws://` não oferece TLS, limite esse modo a uma rede privada confiável e
use um token não previsível. LiteLLM continua exposto apenas em loopback.

O cadastro AWS/modelo e a verificação de custos ficam no
[runbook do LiteLLM](nova-litellm-runbook.md).
