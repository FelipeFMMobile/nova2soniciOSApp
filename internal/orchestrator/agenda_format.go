package orchestrator

import (
	"encoding/json"
	"strings"
	"time"

	"stsmodel.local/poc/internal/mcp"
)

// Validate wire formatting only. Never guess a date or grant permission to mutate.
func agendaFormatError(args json.RawMessage) *mcp.Result {
	var values map[string]json.RawMessage
	if json.Unmarshal(args, &values) != nil {
		return nil
	}
	fields := map[string]string{}
	for _, field := range []string{"start", "end"} {
		raw, exists := values[field]
		if !exists {
			continue
		} // Missing required fields belong to schema validation.
		var text string
		if json.Unmarshal(raw, &text) != nil {
			fields[field] = "Deve ser uma string RFC3339, não objeto, número ou null."
			continue
		}
		value, err := time.Parse(time.RFC3339, text)
		if err != nil || value.Nanosecond() != 0 || strings.ContainsAny(text, ".,") {
			fields[field] = "Use data completa com ano, T, segundos e offset explícito (ou Z), sem frações."
		}
	}
	if len(fields) == 0 {
		return nil
	}
	r := mcp.TextResult(map[string]any{
		"status": "error", "code": "invalid_datetime_format", "fields": fields,
		"expected_format": "AAAA-MM-DDTHH:MM:SS±HH:MM (ou Z)",
		"example":         map[string]string{"start": "2026-09-18T10:00:00-03:00", "end": "2026-09-18T11:00:00-03:00"},
		"instruction":     "Nenhuma ação executada. Converta os campos indicados para RFC3339 a partir dos dados do usuário e chame novamente a ferramenta com os argumentos corrigidos. O exemplo ilustra formato, não valores padrão. Se faltar ano, horário, duração ou fuso, pergunte ao usuário; não adivinhe. Não peça confirmação genérica para corrigir formatação e não anuncie sucesso.",
	}, true)
	return &r
}
