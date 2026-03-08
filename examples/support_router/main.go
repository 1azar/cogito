package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/schema"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/workflow"
)

const (
	intentCertUserID = "cert_uid_lookup"
	intentManual     = "manual"
)

type SupportState struct {
	UserQuestion string
	Intent       string
	RouteReason  string
	Reply        string
}

type RouterDecision struct {
	Intent     string  `json:"intent"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

func main() {

	lookupTool, err := tool.Func(
		"get_user_id_by_cert_activation_key",
		"Find user ID by certificate activation key",
		mockGetUserIDByCertActivationKey,
	)
	if err != nil {
		panic(err)
	}

	specialistAgent := agent.NewAgent[struct{}](&specialistMockLLM{}).
		WithController(react.New[struct{}](react.Config{MaxSteps: 4})).
		WithMemory(buffer.New(20)).
		WithTools(lookupTool).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `Ты специалист поддержки сервиса подарочных сертификатов.

Описание системы:
- Пользователь может купить подарочный сертификат.
- После покупки выдается код активации сертификата.
- Этот код можно подарить другому человеку.
- Получатель вводит код активации и активирует сертификат.
- После активации деньги зачисляются на счет получателя.
- Каждый сертификат может быть активирован только один раз.

Твоя единственная задача:
— определить user_id пользователя, который активировал сертификат,
по ключу активации сертификата.

Язык:
- Отвечай только на русском языке.

Формат ключа:
- Ключ активации имеет формат: AAAA-AAAA-AAAA-AAAA
- Это 4 группы букв или цифр, разделённые дефисами.
- Пример: ABCD-1234-EFGH-5678

Критически важное правило (приоритет №1):

ЕСЛИ в сообщении найден хотя бы один ключ формата AAAA-AAAA-AAAA-AAAA —
считай запрос относящимся к своей задаче и обрабатывай его,
даже если вопрос сформулирован неявно или содержит лишнюю информацию.

В сообщениях могут встречаться:
- номера ЛК
- получатель
- даритель
- номера клиентов
- описание ошибки

Игнорируй эту информацию. Тебя интересуют только ключи сертификатов.

Извлечение ключей:
- Найди все строки формата AAAA-AAAA-AAAA-AAAA в тексте.
- Ключ может быть:
  - в отдельной строке
  - после текста
  - после слов "код активации", "сертификат", "номер сертификата"
  - внутри длинного сообщения.

В одном обращении может быть несколько ключей.

Логика работы:

1. Найди все ключи формата AAAA-AAAA-AAAA-AAAA.

2. Если найден хотя бы один ключ:
   - для каждого ключа вызови функцию get_user_id_by_cert_activation_key.

3. Если ключ один:
   - сообщи user_id активатора.

4. Если ключей несколько:
   - выведи результат для каждого ключа отдельной строкой.

Пример ответа:

Ключ AAAA-BBWS-RVSL-FCEE — активирован пользователем user_id: XXXX  
Ключ F2ER-X3Z8-QNETH-SDFS — активирован пользователем user_id: YYYY

5. Если пользователь по ключу не найден:
   - сообщи об этом и предложи передать запрос оператору.

6. Если ключа нет:
   - попроси пользователя указать ключ активации сертификата
     в формате AAAA-AAAA-AAAA-AAAA.

Ограничения:
- Не отвечай на вопросы вне этой задачи.
- Не обсуждай оплату, возвраты, ошибки системы.
- Единственная задача — определить user_id активатора по ключу сертификата.`
		})

	routerAgent := agent.NewAgent[struct{}](&routerMockLLM{}).
		WithController(simple.New[struct{}]()).
		WithMemory(buffer.New(5)).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `Ты LLM-роутер обращений поддержки.

Доступные интенты:
1) cert_uid_lookup — вопрос относится к активации подарочного сертификата и/или поиску пользователя, активировавшего сертификат.
2) manual — все остальные обращения.

Верни строго JSON без markdown:
{"intent":"cert_uid_lookup|manual","reason":"...","confidence":0.0}

confidence должен быть в диапазоне [0.0, 1.0].`
		})

	g := workflow.NewGraph[*SupportState]()

	g.AddNode("llm_router", workflow.NewAgentNode(
		"llm_router",
		routerAgent,
		func(st workflow.State) (string, error) {
			s := st.(*SupportState)
			return s.UserQuestion, nil
		},
		func(st workflow.State, output string) (workflow.State, error) {
			s := st.(*SupportState)
			decision, err := parseRouterDecision(output)
			if err != nil {
				s.Intent = intentManual
				s.RouteReason = "invalid router output"
				return s, nil
			}

			s.Intent = decision.Intent
			s.RouteReason = fmt.Sprintf("%s (confidence=%.2f)", decision.Reason, decision.Confidence)
			return s, nil
		},
		"LLM router chooses specialist intent",
	))

	g.AddNode("cert_uid_specialist", workflow.NewAgentNodeWithField(
		"cert_uid_specialist",
		specialistAgent,
		func(s *SupportState) string { return s.UserQuestion },
		func(s *SupportState, output string) { s.Reply = output },
		"Specialist: user id by certificate activation key",
	))

	g.AddNode("manual_fallback", workflow.FunctionNode(
		"manual_fallback",
		func(ctx context.Context, st workflow.State) (workflow.State, error) {
			s := st.(*SupportState)
			s.Reply = "Этот запрос вне зоны автоответа."
			return s, nil
		},
		"Route to manual support",
	))

	g.AddConditionalEdge("llm_router", func(s *SupportState) (string, error) {
		if s.Intent == intentCertUserID {
			return intentCertUserID, nil
		}
		return intentManual, nil
	}, map[string]string{
		intentCertUserID: "cert_uid_specialist",
		intentManual:     "manual_fallback",
	})

	g.AddEdge("cert_uid_specialist", workflow.EndNode)
	g.AddEdge("manual_fallback", workflow.EndNode)
	g.SetEntry("llm_router")

	examples := []string{
		"Нужен user id по ключу активации сертификата CERT-ABC-123",
		"У меня не работает оплата, что делать?",
		"Подскажи user id по сертификату", // no key -> specialist asks for key
		"Получатель 32674527\nДаритель неизвестен кл\nE6Y7-XXXX-KNNH-FFFS где активирован?",
		"303279571\nAAAA-BBWS-RVSL-FCEE\nF2ER-X3Z8-QNETH-SDFS\nгде актвированы?",
		"45801172\nОбратился даритель\nXLLE-DU9M-62BS-ASDD\nПолучатель 6217881\nУ получателя активировать не получилось, но у дарителя отображается как активированный.",
		"Лк 26133762\nНомер сертификата: YVFT-KG8W-GSDD-KEQE\nНомер сертификата: 7B9E-ASDW-9L55-SF7U\nВопрос/ошибка: сертификаты уже активированы, обращается даритель.\nГде были активированы?",
		"ЛК 41342546\nКод активации : DDDA-PE26-8J9K-9G9B\nКлиент не знает номер дарителя, при активации сертификата ошибка, что он уже активирован\nПодскажите, пожалуйста, можем ли проверить, где был ранее активирован сертификат?",
	}

	ctx := context.Background()
	for i, q := range examples {
		state := &SupportState{UserQuestion: q}
		result, runErr := g.Run(ctx, state)
		if runErr != nil {
			fmt.Printf("[%d] error: %v\n", i+1, runErr)
			continue
		}

		fmt.Printf("[%d] Q: %s\n", i+1, result.UserQuestion)
		fmt.Printf("    intent: %s (%s)\n", result.Intent, result.RouteReason)
		fmt.Printf("    A: %s\n\n", result.Reply)
	}
}

func classifyIntent(q string) (intent string, reason string) {
	s := strings.ToLower(q)
	keys := extractActivationKeys(q)

	hasUserID := strings.Contains(s, "user id") ||
		strings.Contains(s, "userid") ||
		strings.Contains(s, "юзер") ||
		strings.Contains(s, "пользовател")

	hasCert := strings.Contains(s, "сертифик") ||
		strings.Contains(s, "certificate") ||
		strings.Contains(s, "cert")

	hasActivationKey := strings.Contains(s, "ключ") ||
		strings.Contains(s, "activation key") ||
		strings.Contains(s, "activation")

	hasActivationContext := strings.Contains(s, "активир") ||
		strings.Contains(s, "активац") ||
		strings.Contains(s, "актвир") ||
		strings.Contains(s, "дарител") ||
		strings.Contains(s, "получател") ||
		strings.Contains(s, "код") ||
		strings.Contains(s, "where activated")

	if len(keys) > 0 && (hasCert || hasActivationContext || hasActivationKey || hasUserID) {
		return intentCertUserID, "activation key detected in support context"
	}

	if hasUserID && hasCert && hasActivationKey {
		return intentCertUserID, "matched certificate user_id lookup pattern"
	}

	if hasUserID && hasCert {
		return intentCertUserID, "matched certificate+user_id pattern"
	}

	if hasCert && hasActivationContext {
		return intentCertUserID, "matched activation investigation pattern"
	}

	return intentManual, "out of specialist scope"
}

func parseRouterDecision(raw string) (RouterDecision, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var d RouterDecision
	if err := json.Unmarshal([]byte(clean), &d); err != nil {
		return RouterDecision{}, err
	}

	if d.Intent != intentCertUserID && d.Intent != intentManual {
		return RouterDecision{}, fmt.Errorf("unknown intent: %s", d.Intent)
	}

	if d.Confidence < 0 {
		d.Confidence = 0
	}
	if d.Confidence > 1 {
		d.Confidence = 1
	}

	if strings.TrimSpace(d.Reason) == "" {
		d.Reason = "router decision"
	}

	return d, nil
}

var certLikeKeyRegexp = regexp.MustCompile(`(?i)\b[a-z0-9]{4}(?:-[a-z0-9]{4}){3}\b`)
var legacyKeyRegexp = regexp.MustCompile(`(?i)\bcert-[a-z0-9-]+\b`)

type CertLookupInput struct {
	ActivationKey string `json:"activation_key"`
}

type CertLookupOutput struct {
	ActivationKey string `json:"activation_key"`
	UserID        string `json:"user_id"`
	Found         bool   `json:"found"`
}

func mockGetUserIDByCertActivationKey(ctx context.Context, in CertLookupInput) (CertLookupOutput, error) {
	if strings.TrimSpace(in.ActivationKey) == "" {
		return CertLookupOutput{}, errors.New("activation_key is required")
	}

	db := map[string]string{
		"CERT-ABC-123": "user_1001",
		"CERT-XYZ-777": "user_2042",
	}
	_ = db

	//uid, ok := db[strings.ToUpper(strings.TrimSpace(in.ActivationKey))]
	//if !ok {
	//	return CertLookupOutput{
	//		ActivationKey: in.ActivationKey,
	//		Found:         false,
	//	}, nil
	//}

	uid := strconv.Itoa(rand.Int())

	return CertLookupOutput{
		ActivationKey: in.ActivationKey,
		UserID:        uid,
		Found:         true,
	}, nil
}

type specialistMockLLM struct{}

type routerMockLLM struct{}

func (m *routerMockLLM) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	msgs := req.Messages
	if len(msgs) == 0 {
		decision, _ := json.Marshal(RouterDecision{
			Intent:     intentManual,
			Reason:     "empty conversation",
			Confidence: 0.1,
		})
		return &llm.Response{Text: string(decision)}, nil
	}

	question := msgs[len(msgs)-1].Content
	intent, reason := classifyIntent(question)

	confidence := 0.9
	if intent == intentManual {
		confidence = 0.7
	}

	decision, _ := json.Marshal(RouterDecision{
		Intent:     intent,
		Reason:     reason,
		Confidence: confidence,
	})

	return &llm.Response{Text: string(decision)}, nil
}

func (m *specialistMockLLM) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	msgs := req.Messages
	if len(msgs) == 0 {
		return &llm.Response{Text: ""}, nil
	}

	last := msgs[len(msgs)-1]
	if last.Role == schema.RoleTool {
		responses := collectTrailingToolOutputs(msgs)
		if len(responses) == 0 {
			return &llm.Response{Text: "Не удалось обработать ответ инструмента. Передаю оператору."}, nil
		}

		if len(responses) == 1 {
			return &llm.Response{Text: responses[0]}, nil
		}

		return &llm.Response{Text: strings.Join(responses, "\n")}, nil
	}

	question := last.Content
	keys := extractActivationKeys(question)
	if len(keys) == 0 {
		return &llm.Response{
			Text: "Уточните, пожалуйста, код активации сертификата в формате AAAA-AAAA-AAAA-AAAA.",
		}, nil
	}

	toolCalls := make([]schema.ToolCall, 0, len(keys))
	for i, key := range keys {
		args, _ := json.Marshal(CertLookupInput{ActivationKey: key})
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:        fmt.Sprintf("call_cert_lookup_%d", i+1),
			Name:      "get_user_id_by_cert_activation_key",
			Arguments: args,
		})
	}

	return &llm.Response{
		ToolCalls: toolCalls,
	}, nil
}

func extractActivationKeys(question string) []string {
	matches := certLikeKeyRegexp.FindAllString(question, -1)
	matches = append(matches, legacyKeyRegexp.FindAllString(question, -1)...)

	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	keys := make([]string, 0, len(matches))
	for _, match := range matches {
		normalized := strings.ToUpper(strings.TrimSpace(match))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		keys = append(keys, normalized)
	}

	return keys
}

func collectTrailingToolOutputs(msgs []schema.Message) []string {
	outputs := make([]string, 0)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != schema.RoleTool {
			break
		}

		var envelope struct {
			Status string          `json:"status"`
			Output json.RawMessage `json:"output"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}

		if err := json.Unmarshal([]byte(msgs[i].Content), &envelope); err != nil {
			// Backward compatibility with legacy raw tool output format in examples.
			var legacy CertLookupOutput
			if legacyErr := json.Unmarshal([]byte(msgs[i].Content), &legacy); legacyErr != nil {
				continue
			}
			if legacy.Found {
				outputs = append(outputs, fmt.Sprintf("Ключ %s — активирован пользователем user_id: %s.", legacy.ActivationKey, legacy.UserID))
				continue
			}
			outputs = append(outputs, fmt.Sprintf("Ключ %s — пользователь не найден. Могу передать запрос оператору.", legacy.ActivationKey))
			continue
		}

		if envelope.Status == "error" {
			if envelope.Error != nil && envelope.Error.Message != "" {
				outputs = append(outputs, fmt.Sprintf("Ошибка при проверке сертификата: %s. Могу передать запрос оператору.", envelope.Error.Message))
			} else {
				outputs = append(outputs, "Ошибка при проверке сертификата. Могу передать запрос оператору.")
			}
			continue
		}

		var toolOut CertLookupOutput
		if err := json.Unmarshal(envelope.Output, &toolOut); err != nil {
			continue
		}

		if toolOut.Found {
			outputs = append(outputs, fmt.Sprintf("Ключ %s — активирован пользователем user_id: %s.", toolOut.ActivationKey, toolOut.UserID))
			continue
		}

		outputs = append(outputs, fmt.Sprintf("Ключ %s — пользователь не найден. Могу передать запрос оператору.", toolOut.ActivationKey))
	}

	for i, j := 0, len(outputs)-1; i < j; i, j = i+1, j-1 {
		outputs[i], outputs[j] = outputs[j], outputs[i]
	}

	return outputs
}
