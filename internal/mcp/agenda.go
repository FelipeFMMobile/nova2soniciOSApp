package mcp

import (
	"context"
	"encoding/json"
	"io"
	"stsmodel.local/poc/internal/storage"
)

func AgendaTools() []Tool {
	interval := `"start":{"type":"string","format":"date-time","minLength":20,"maxLength":35,"description":"Início em RFC3339: AAAA-MM-DDTHH:MM:SS com offset explícito. Em America/Sao_Paulo, 20 de maio de 2030 às 09:00 corresponde a 2030-05-20T09:00:00-03:00. Envie a data completa, não texto por extenso. O exemplo não é uma data padrão; esclareça ano ou data ausente.","examples":["2030-05-20T09:00:00-03:00"]},"end":{"type":"string","format":"date-time","minLength":20,"maxLength":35,"description":"Fim exclusivo em RFC3339 com offset explícito, posterior a start. Para o início do exemplo e duração de uma hora, envie 2030-05-20T10:00:00-03:00. O exemplo não é um valor padrão; use a duração informada pelo usuário ou peça esclarecimento.","examples":["2030-05-20T10:00:00-03:00"]}`
	return []Tool{
		{Name: "agenda.list_slots", Description: "Consulta horários disponíveis na agenda fictícia local, America/Sao_Paulo, dias úteis 09–18h. Esclareça datas e duração ambíguas.", InputSchema: json.RawMessage(`{"type":"object","properties":{` + interval + `,"duration_minutes":{"type":"integer","minimum":15,"maximum":240,"multipleOf":15}},"required":["start","end","duration_minutes"],"additionalProperties":false}`)},
		{Name: "agenda.list_events", Description: "Consulta eventos fictícios por intervalo absoluto. start/end são UTC. Para falar horários ao usuário, use start_local/end_local, calculados em America/Sao_Paulo; não chame o valor UTC de horário de Brasília.", InputSchema: json.RawMessage(`{"type":"object","properties":{` + interval + `},"required":["start","end"],"additionalProperties":false}`)},
		{Name: "agenda.create_event", Description: "Cria evento somente após pedido explícito para agendar. Esclareça horário, data, ano, duração ou título ausente/ambíguo. Envie start e end no formato RFC3339 AAAA-MM-DDTHH:MM:SS±HH:MM, com offset explícito; nunca envie datas por extenso ou apenas DD/MM/AAAA nos argumentos. Exemplo: para pocket em 20 de maio de 2030 às 09:00, com duração de uma hora em America/Sao_Paulo, envie {\"title\":\"pocket\",\"start\":\"2030-05-20T09:00:00-03:00\",\"end\":\"2030-05-20T10:00:00-03:00\"}. O exemplo não define valores padrão. Calcule end a partir da duração informada; end deve ser posterior a start. Para datas relativas, use a data atual e o fuso fornecidos na sessão. Não peça que o usuário dite RFC3339: formate os argumentos antes de chamar a tool. Se receber invalid_datetime ou invalid_arguments, siga instruction e corrija os campos. Pedidos por voz com datas faladas ou relativas geram confirmation_required: leia o título, data completa com ano, início, fim e fuso da proposta retornada. Peça exatamente Confirmo agendar em um novo turno, não apenas sim. Depois chame novamente com os MESMOS argumentos. Uma confirmação antiga não autoriza dados alterados. Nunca anuncie sucesso antes de status created.", InputSchema: json.RawMessage(`{"type":"object","properties":{` + interval + `,"title":{"type":"string","minLength":1,"maxLength":256}},"required":["start","end","title"],"additionalProperties":false}`)},
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
