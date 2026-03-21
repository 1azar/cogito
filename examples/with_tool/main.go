package main

import (
	"context"
	"fmt"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/llm/openai"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/tool"
	"github.com/1azar/cogito/tool/tools"
)

type State struct{}

func main() {

	//llm, err := openai.New(openai.Config{
	//	APIKey:  os.Getenv("OPENAI_API_KEY"),
	//	BaseURL: os.Getenv("OPENAI_BASE_URL"),
	//	Model:   os.Getenv("OPENAI_MODEL"),
	//	Timeout: 0,
	//})
	//llm, err := ollama.New(ollama.Config{
	//	BaseURL: "http://localhost:11434/v1",
	//	//Model:   "qwen2.5-coder:7b",
	//	//Model: "deepseek-coder:6.7b",
	//	Model: "qwen3:4b",
	//})
	llm, err := openai.New(openai.Config{
		APIKey:  "ollama",
		BaseURL: "http://localhost:11434/v1",
		//Model:   "qwen2.5-coder:7b",
		//Model: "deepseek-coder:6.7b",
		Model: "qwen3:4b",
	})
	if err != nil {
		panic(err)
	}

	weatherTool, err := tool.Func("get_weather", "Get the current weather for a city", GetWeather)
	if err != nil {
		panic(err)
	}
	_ = weatherTool

	ag := agent.NewAgent[State](llm).
		WithController(react.New[State](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(10)).
		WithTools(
			weatherTool,
			tools.CalculatorTool,
			tools.CurrentTimeTool,
		)

	out, err := ag.Run(context.Background(), "Can you give me a sum of temperatures in Capital of USA and capital of Tatarstan republic")
	//out, err := ag.Run(context.Background(), "What time is it in Ufa?")
	if err != nil {
		panic(err)
	}

	fmt.Println(out)
}

// tool
type WeatherInput struct {
	City string `json:"city"`
}

type WeatherOutput struct {
	Temp int `json:"temp"`
}

func GetWeather(ctx context.Context, in WeatherInput) (WeatherOutput, error) {
	//return WeatherOutput{}, errors.New("not implemented")
	fmt.Println("GetWeather tool call")
	return WeatherOutput{Temp: 9}, nil
}
