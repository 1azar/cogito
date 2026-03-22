package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/controller/simple"
	"github.com/1azar/cogito/llm/openai"
	"github.com/1azar/cogito/memory/buffer"
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
	llm, err := openai.New(openai.Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   os.Getenv("OPENAI_MODEL"),
		Timeout: 0,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create LLM: %v\n", err)
		os.Exit(1)
	}

	lookupActivatorByKeyTool, err := tool.Func(
		"get_activator_user_id_by_cert_activation_key",
		"Find activator user ID by certificate activation key",
		mockGetUserIDByCertActivationKey,
	)
	if err != nil {
		panic(err)
	}

	specialistAgent := agent.NewAgent[struct{}](llm).
		WithController(react.New[struct{}](react.Config{MaxSteps: 4})).
		WithMemory(buffer.New(20)).
		WithTools(lookupActivatorByKeyTool).
		WithPromptFunc(func(ctx context.Context, state *struct{}) string {
			return `Ты специалист поддержки сервиса подарочных сертификатов.

Описание системы:
- Пользователь может купить подарочный сертификат.
- После покупки выдается код активации сертификата.
- Этот код можно подарить другому человеку.
- Получатель вводит код активации и активирует сертификат.
- После активации деньги зачисляются на счет получателя.
- Каждый сертификат может быть активирован только один раз.
- для получения кода активации покупатель должен открыть pdf файл с сертификатом в лично кабинете
- при каждом открытии pdf код активации обновляется
- одновременно у одного сертификата может быть до 20 кодов активации

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

	routerAgent := agent.NewAgent[struct{}](llm).
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

	g.AddNode(workflow.NewAgentNode(
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

	g.AddNode(workflow.NewAgentNodeWithField(
		"cert_uid_specialist",
		specialistAgent,
		func(s *SupportState) string { return s.UserQuestion },
		func(s *SupportState, output string) { s.Reply = output },
		"Specialist: user id by certificate activation key",
	))

	g.AddNode(workflow.FunctionNode(
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
		//"Нужен user id по ключу активации сертификата CERT-ABC-123",
		//"У меня не работает оплата, что делать?",
		//"Подскажи user id по сертификату", // no key -> specialist asks for key
		//"Получатель 32674527\nДаритель неизвестен кл\nE6Y7-XXXX-KNNH-FFFS где активирован?",
		//"303279571\nAAAA-BBWS-RVSL-FCEE\nF2ER-X3Z8-QNETH-SDFS\nгде актвированы?",
		//"45801172\nОбратился даритель\nXLLE-DU9M-62BS-ASDD\nПолучатель 6217881\nУ получателя активировать не получилось, но у дарителя отображается как активированный.",
		//"Лк 26133762\nНомер сертификата: YVFT-KG8W-GSDD-KEQE\nНомер сертификата: 7B9E-ASDW-9L55-SF7U\nВопрос/ошибка: сертификаты уже активированы, обращается даритель.\nГде были активированы?",
		//"ЛК 41342546\nКод активации : DDDA-PE26-8J9K-9G9B\nКлиент не знает номер дарителя, при активации сертификата ошибка, что он уже активирован\nПодскажите, пожалуйста, можем ли проверить, где был ранее активирован сертификат?",
		"лк 123559674\nошибка при активации сертификата-уже активирован\nномер телефона дарителя не может уточнить\nE9PN-8PPU-AWU4-MHTV\nможем проверить?",
		"лк 37707823\nXCPX-BJ8F-6QSE-XYKJ\nМожно проверить где активирован? Номер дарителя не знает. ",
		"104448657 \nMQVW-XJTX-5433-PF3T\n\nгде активированЮ?",
		"15325349\n845Q-XPU5-KYKQ-YH32\nможем уточнить где активирован, номер дарителя кл-т не знает, на работе дарили",
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
		"CERT-ABC-123":        "user_1001",
		"7B9E-ASDW-9L55-SF7U": "user_2042",
	}
	_ = db

	//uid, ok := db[strings.ToUpper(strings.TrimSpace(in.ActivationKey))]
	//if !ok {
	//	return CertLookupOutput{
	//		ActivationKey: in.ActivationKey,
	//		Found:         false,
	//	}, nil
	//}

	s := rand.Intn(8)
	if s == 1 {
		return CertLookupOutput{
			ActivationKey: in.ActivationKey,
			Found:         false,
		}, nil
	}

	uid := strconv.Itoa(rand.Intn(300_000_000))

	return CertLookupOutput{
		ActivationKey: in.ActivationKey,
		UserID:        uid,
		Found:         true,
	}, nil
}
