package main

import (
	"context"
	"fmt"
	"os"

	"github.com/1azar/cogito/agent"
	"github.com/1azar/cogito/controller/react"
	"github.com/1azar/cogito/llm/openai"
	"github.com/1azar/cogito/memory/buffer"
	"github.com/1azar/cogito/tool"
)

type State struct{}

func main() {

	llm, err := openai.New(openai.Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   os.Getenv("OPENAI_MODEL"),
		Timeout: 0,
	})
	if err != nil {
		panic(err)
	}

	weatherTool, err := tool.Func("get_weather", "Get the current weather for a city", GetWeather)
	if err != nil {
		panic(err)
	}

	ag := agent.NewAgent[State](llm).
		WithController(react.New[State](react.Config{MaxSteps: 5})).
		WithMemory(buffer.New(10)).
		WithTools(weatherTool)

	out, err := ag.Run(context.Background(), "What's the weather in capital of Bashkortostan Republic?")
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
	return WeatherOutput{Temp: 9}, nil
}
