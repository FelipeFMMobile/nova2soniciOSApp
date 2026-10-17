package account

import "encoding/json"

// Fixed catalog; no arbitrary URL, script, component or model-produced action.
func loginSurface(transaction string) json.RawMessage {
	messages := []any{
		map[string]any{"version": "v0.9.1", "deleteSurface": map[string]any{"surfaceId": "account-login"}},
		map[string]any{"version": "v0.9.1", "createSurface": map[string]any{"surfaceId": "account-login", "catalogId": "urn:sts:a2ui:account:v1"}},
		map[string]any{"version": "v0.9.1", "updateComponents": map[string]any{"surfaceId": "account-login", "components": []any{
			map[string]any{"id": "root", "component": "Column", "children": []string{"instructions", "login"}},
			map[string]any{"id": "instructions", "component": "Text", "text": "Entre com sua senha e o código recebido por e-mail na tela segura."},
			map[string]any{"id": "login", "component": "Button", "child": "label", "action": map[string]any{"event": map[string]any{"name": "account.authenticate", "context": map[string]string{"transaction_id": transaction}}}},
			map[string]any{"id": "label", "component": "Text", "text": "Autenticar"},
		}}},
	}
	b, _ := json.Marshal(messages)
	return b
}
