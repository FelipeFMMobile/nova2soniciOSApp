package account

import (
	"strings"
	"stsmodel.local/voicewire"
	"unicode"
)

const welcomeMessage = "Sou agente STS para logar sua conta, deseja continuar?"
const welcomeRequest = "Inicie esta sessão anônima falando apenas: Sou agente STS para logar sua conta, deseja continuar? Depois aguarde a resposta do usuário, sem chamar ferramentas."
const connectedMessage = "Conta conectada com sucesso, agora você pode consultar e editar informações da sua conta"
const connectedRequest = "O gateway confirmou a sessão autenticada. Fale somente esta confirmação, sem chamar ferramentas: " + connectedMessage
const connectedTurn = "account-connected"
const welcomeOperation = "account-login-consent"

func welcomeEvents() []protocol.Event {
	return []protocol.Event{
		{Type: protocol.Transcript, TurnID: welcomeOperation, Role: "ASSISTANT", Stage: "FINAL", Text: welcomeMessage},
		{Type: protocol.ToolConfirmation, TurnID: welcomeOperation, Tool: &protocol.Tool{OperationID: welcomeOperation, Name: "identity.begin_login"}, Message: welcomeMessage},
	}
}

// Only a completed user utterance answers the invitation. Ambiguity stays pending.
func welcomeAnswer(text string) int {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	switch text {
	case "sim", "sim quero", "sim pode", "sim pode continuar", "sim desejo continuar", "sim quero continuar", "pode continuar", "quero continuar", "continuar", "vamos", "claro":
		return 1
	case "não", "nao", "não quero", "nao quero", "não quero continuar", "nao quero continuar", "não desejo continuar", "nao desejo continuar", "agora não", "agora nao", "cancelar", "encerre", "encerrar":
		return -1
	}
	return 0
}
