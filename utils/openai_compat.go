package utils

import (
	"encoding/json"
	"strings"
)

// AdaptChatBody — chat/completions gövdesini seçili modele uyarlar.
// gpt-5 / o-serisi akıl yürütme modelleri temperature, top_p, ceza parametreleri ve max_tokens kabul etmez;
// bunlar çıkarılır, max_tokens → max_completion_tokens olur (akıl yürütme payı eklenir).
// Diğer modellerde gövde aynen döner.
func AdaptChatBody(body []byte) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	model, _ := m["model"].(string)
	if !isReasoningModel(model) {
		return body
	}

	for _, k := range []string{"temperature", "top_p", "presence_penalty", "frequency_penalty", "logprobs", "top_logprobs"} {
		delete(m, k)
	}
	limit := 16000
	if v, ok := m["max_tokens"].(float64); ok && v > 0 {
		limit = int(v) + 4000 // akıl yürütme token'ları da bu sınırdan düşülür
		delete(m, "max_tokens")
	}
	if _, ok := m["max_completion_tokens"]; !ok {
		m["max_completion_tokens"] = limit
	}
	if _, ok := m["reasoning_effort"]; !ok && !strings.Contains(strings.ToLower(model), "chat") {
		m["reasoning_effort"] = "low"
	}

	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}
