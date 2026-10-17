package account

import (
	"errors"
	"net/url"
	"os"
	"strings"
	orchestrator "stsmodel.local/actionruntime"
	"time"
)

type Config struct {
	Provider, PromptFile, LiteLLMURL, LiteLLMAPIKey, LiteLLMModel, MCPContextSecret string
	NovaMaxSessionAge, MCPTimeout, TurnTimeout                                      time.Duration
}

func (c Config) Validate() error {
	u, e := url.Parse(c.LiteLLMURL)
	if e != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" || c.LiteLLMAPIKey == "" || c.PromptFile == "" {
		return errors.New("invalid account configuration")
	}
	return nil
}
func Load() Config {
	return Config{Provider: env("STS_PROVIDER", "nova"), PromptFile: os.Getenv("STS_AGENT_PROMPT_FILE"), LiteLLMURL: os.Getenv("STS_LITELLM_URL"), LiteLLMAPIKey: os.Getenv("STS_LITELLM_API_KEY"), LiteLLMModel: env("STS_LITELLM_REALTIME_MODEL", "nova-sonic"), MCPContextSecret: os.Getenv("STS_MCP_CONTEXT_SECRET"), NovaMaxSessionAge: 7 * time.Minute, MCPTimeout: 10 * time.Second, TurnTimeout: 30 * time.Second}
}
func env(k, d string) string {
	if s := os.Getenv(k); s != "" {
		return s
	}
	return d
}
func LoadPrompt(path string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil || len(b) == 0 || len(b) > 65536 {
		return "", errors.New("account prompt unavailable")
	}
	return strings.TrimSpace(string(b)), nil
}

var policies = map[string]orchestrator.Profile{
	"identity.begin_login":  {Policy: "explicit_intent", Direct: true},
	"identity.login_status": {Policy: "read_only"},
	"identity.logout":       {Policy: "confirm_later", ConfirmationPhrase: "confirmo sair", AllowUIConfirmation: true},
	"profile.get_me":        {Policy: "read_only"},
	"profile.update_me":     {Policy: "confirm_later", ConfirmationPhrase: "confirmo alterar meus dados", AllowUIConfirmation: true},
}
var descriptions = map[string]string{
	"identity.begin_login":  "Apresentar autenticação segura no app. Nunca solicitar senha ou código por voz.",
	"identity.login_status": "Consultar se a sessão atual está autenticada.",
	"identity.logout":       "Encerrar minha sessão após confirmação.",
	"profile.get_me":        "Consultar apenas meus dados autenticados: nome, idade, cidade, gênero e profissão.",
	"profile.update_me":     "Atualizar meus dados com a versão consultada e alterações confirmadas. Não inferir gênero ou idade.",
}
