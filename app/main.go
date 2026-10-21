package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	var messages []openai.ChatCompletionMessageParamUnion

	if prompt == "" {
		panic("Prompt must not be empty")
	}

	messages = append(messages, openai.UserMessage(prompt))

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://openrouter.ai/api/v1"
	}

	if apiKey == "" {
		panic("Env variable OPENROUTER_API_KEY not found")
	}

	fmt.Fprintln(os.Stderr, "Logs from your program will appear here!")
	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl))

	for {

		resp, err := client.Chat.Completions.New(context.Background(),
			openai.ChatCompletionNewParams{
				Model:    "anthropic/claude-haiku-4.5",
				Messages: messages,
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
					openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
						Name:        "Write",
						Description: openai.String("Write content to a file"),
						Parameters: openai.FunctionParameters{
							"type": "object",
							"properties": map[string]any{
								"file_path": map[string]any{
									"type":        "string",
									"description": "The path to the file to write to",
								},
								"content": map[string]any{
									"type":        "string",
									"description": "The content to write to the file",
								},
							},
							"required": []string{"file_path", "content"},
						},
					}),
					openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
						Name:        "Bash",
						Description: openai.String("Execute a shell command"),
						Parameters: openai.FunctionParameters{
							"type": "object",
							"properties": map[string]any{
								"command": map[string]any{
									"type":        "string",
									"description": "The path to the file to write to",
								},
								"content": map[string]any{
									"type":        "string",
									"description": "The command to execute",
								},
							},
							"required": []string{"command"},
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

		messages = append(messages, resp.Choices[0].Message.ToParam())

		tool_calls := resp.Choices[0].Message.ToolCalls
		if len(tool_calls) == 0 {
			fmt.Print(resp.Choices[0].Message.Content)
			break
		} else {
			for _, tool_call := range tool_calls {

				result, err := handleTool(tool_call)
				if err == nil {
					messages = append(messages, openai.ToolMessage(result, tool_call.ID))
				}
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
	case "Write":
		err := os.WriteFile(tool_args_map["file_path"], []byte(tool_args_map["content"]), 0644)
		if err != nil {
			return "", err
		}
		return "Write sucessfull", nil
	case "Bash":
		cmd := exec.Command("bash", "-c", tool_args_map["command"])
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), err
		}
		return string(out), nil
	default:
		fmt.Print("Unknow tool")
	}
	return "Unknow tool call", nil
}
