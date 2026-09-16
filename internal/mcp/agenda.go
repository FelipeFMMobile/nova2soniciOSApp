package mcp

import (
	"context"
	"encoding/json"
	"io"
	"stsmodel.local/poc/internal/storage"
)

func AgendaTools() []Tool {
	interval := `"start":{"type":"string","minLength":20,"maxLength":35,"description":"Instante ISO 8601 RFC3339 com offset explícito, por exemplo 2030-05-20T09:00:00-03:00"},"end":{"type":"string","minLength":20,"maxLength":35,"description":"Fim exclusivo RFC3339 com offset explícito"}`
	return []Tool{
		{Name: "agenda.list_slots", Description: "Consulta horários disponíveis na agenda fictícia local, America/Sao_Paulo, dias úteis 09–18h. Esclareça datas e duração ambíguas.", InputSchema: json.RawMessage(`{"type":"object","properties":{` + interval + `,"duration_minutes":{"type":"integer","minimum":15,"maximum":240,"multipleOf":15}},"required":["start","end","duration_minutes"],"additionalProperties":false}`)},
		{Name: "agenda.list_events", Description: "Consulta eventos fictícios por intervalo absoluto. Retorna UTC e timezone America/Sao_Paulo.", InputSchema: json.RawMessage(`{"type":"object","properties":{` + interval + `},"required":["start","end"],"additionalProperties":false}`)},
		{Name: "agenda.create_event", Description: "Cria evento somente após pedido explícito para agendar. Esclareça horário, data ou título ausente/ambíguo. Nunca anuncie sucesso antes de status created. Use horários RFC3339 com offset. Nesta POC o host exige data completa numérica e horário explícito no ASR final; se faltar, peça esclarecimento.", InputSchema: json.RawMessage(`{"type":"object","properties":{` + interval + `,"title":{"type":"string","minLength":1,"maxLength":256}},"required":["start","end","title"],"additionalProperties":false}`)},
		{Name: "agenda.cancel_event", Description: "Cancela somente o ID retornado pela consulta e após confirmação posterior. Peça exatamente confirmo cancelar agendamento. Só anuncie cancelamento após status cancelled.", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","minLength":1,"maxLength":128}},"required":["id"],"additionalProperties":false}`)},
	}
}
func ServeAgenda(ctx context.Context, in io.Reader, out io.Writer, s *storage.Agenda) error {
	return ServeTools(ctx, in, out, "sts-agenda", AgendaTools(), func(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) Result {
		var i storage.AgendaInput
		_ = json.Unmarshal(args, &i)
		switch name {
		case "agenda.list_events":
			events, err := s.Events(ctx, i.Start, i.End)
			if err != nil {
				return Failure("invalid_interval_or_storage")
			}
			return TextResult(map[string]any{"status": "ok", "events": events, "timezone": storage.AgendaTimezone}, false)
		case "agenda.list_slots":
			slots, err := s.Slots(ctx, i)
			if err != nil {
				return Failure("invalid_interval_or_storage")
			}
			return TextResult(map[string]any{"status": "ok", "slots": slots, "timezone": storage.AgendaTimezone}, false)
		default:
			data, err := s.MutateAgenda(ctx, key, name, args, confirmed)
			if err != nil {
				code := err.Error()
				switch code {
				case "invalid_interval", "outside_availability", "slot_conflict", "event_not_found", "confirmation_required", "idempotency_conflict", "invalid_title", "invalid_key":
				default:
					code = "mutation_failed"
				}
				return Failure(code)
			}
			return TextResult(data, false)
		}
	})
}
