package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	var messages []any

	if prompt == "" {
		panic("Prompt must not be empty")
	}

	messages = append(messages, map[string]string{"role": "user", "content": prompt})

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://openrouter.ai/api/v1"
	}

	if apiKey == "" {
		panic("Env variable OPENROUTER_API_KEY not found")
	}

	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl))
	resp, err := client.Chat.Completions.New(context.Background(),
		openai.ChatCompletionNewParams{
			Model: "anthropic/claude-haiku-4.5",
			Messages: []openai.ChatCompletionMessageParamUnion{
				{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfString: openai.String(prompt),
						},
					},
				},
			},
			Tools: []openai.ChatCompletionToolUnionParam{
				openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
					Name:        "Read",
					Description: openai.String("Read and return the contents of a file"),
					Parameters: openai.FunctionParameters{
						"type": "object",
						"properties": map[string]any{
							"file_path": map[string]any{
								"type":        "string",
								"description": "The path to the file to read",
							},
						},
						"required": []string{"file_path"},
					},
				}),
			},
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(resp.Choices) == 0 {
		panic("No choices in response")
	}

	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Fprintln(os.Stderr, "Logs from your program will appear here!")

	messages = append(messages, resp)

	tool_calls := resp.Choices[0].Message.ToolCalls
	if len(tool_calls) == 0 {
		fmt.Print(resp.Choices[0].Message.Content)
	} else {
		for _, tool_call := range tool_calls {

			result, err := handleTool(tool_call)
			if err == nil {
				messages = append(messages, map[string]string{"role": "tool", "tool_call_id": tool_call.ID, "content": result})
			}
		}
	}
}

func handleTool(tool_call openai.ChatCompletionMessageToolCallUnion) (string, error) {
	tool_name := tool_call.Function.Name
	tool_args := tool_call.Function.Arguments
	var tool_args_map map[string]string
	args_err := json.Unmarshal([]byte(tool_args), &tool_args_map)

	if args_err != nil {
		return "", args_err
	}

	switch tool_name {
	case "Read":
		data, err := os.ReadFile(tool_args_map["file_path"])
		if err != nil {
			return "", err
		}
		text := string(data)
		return text, nil
	default:
		fmt.Print("Unknow tool")
	}
	return "", nil
}
