package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"gopkg.in/yaml.v3"
)

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM")
	flag.Parse()

	var messages []openai.ChatCompletionMessageParamUnion

	if prompt == "" {
		panic("Prompt must not be empty")
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")
	if baseUrl == "" {
		baseUrl = "https://openrouter.ai/api/v1"
	}

	if apiKey == "" {
		panic("Env variable OPENROUTER_API_KEY not found")
	}

	skill_promt := "You have access to the following skills: /n"
	entries, err := os.ReadDir(".claude/skills/")
	all_skills := map[string]string{}
	if err == nil {
		var skills_headers []string

		skills_headers = append(skills_headers, skill_promt)
		for _, skill := range entries {

			skill_header, skill_body, _ := extractSkillInfo(".claude/skills/" + skill.Name() + "/SKILL.md")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			skills_headers = append(skills_headers, fmt.Sprintf("- %s: %s", skill_header["name"], skill_header["description"]))
			all_skills["/"+skill.Name()] = skill_body
		}
		messages = append(messages, openai.SystemMessage(strings.Join(skills_headers, "\n")))
	}

	messages = append(messages, processPrompt(prompt, all_skills)...)
	fmt.Fprintln(os.Stderr, "debug messages: ", messages)

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
				} else {
					messages = append(messages, openai.ToolMessage(fmt.Sprintf("error: ", err), tool_call.ID))
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

func extractSkillInfo(path string) (map[string]any, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}

	defer f.Close()

	var header []string
	var body []string
	sc := bufio.NewScanner(f)
	started := false
	start_body := false
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "---") && !started {
			started = true
		} else if strings.HasPrefix(line, "---") && started {
			start_body = true
		} else if start_body {
			body = append(body, line)
		} else if started {
			header = append(header, line)
		}
	}

	var skill_header map[string]any

	if err := sc.Err(); err != nil {
		return nil, "", err
	}

	err = yaml.Unmarshal([]byte(strings.Join(header, "\n")), &skill_header)
	if err != nil {
		return nil, "", err
	}

	return skill_header, strings.Join(body, "\n"), nil
}

func processPrompt(prompt string, all_skills map[string]string) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion

	if strings.HasPrefix(prompt, "/") {
		skill_name := strings.Split(prompt, " ")

		args_index := 0
		for index := range skill_name {
			if strings.HasPrefix(prompt, "/") {
				args_index = index
				if _, ok := all_skills[skill_name[index]]; !ok {
					args_index = index
					break
				}
			}
		}

		if args_index > 0 {
			for _, value := range skill_name[:args_index] {
				skill_body, _ := all_skills[value]
				skill_args := strings.Join(skill_name[args_index:], " ")
				var updated_body string
				updated_body = strings.ReplaceAll(skill_body, "$ARGUMENTS", skill_args)
				for index, value := range skill_name[args_index:] {
					arg_position := fmt.Sprintf("$%d", index)
					updated_body = strings.ReplaceAll(updated_body, arg_position, value)
				}

				relative_skill := fmt.Sprintf("Skill: %s (located at .claude/skills/%s) \nPaths in the instructions below are relative to that folder.\n\n", value, value)

				fmt.Fprintln(os.Stderr, "debug: ", relative_skill+updated_body)
				messages = append(messages, openai.UserMessage(relative_skill+updated_body))

			}
		} else if skill_body, ok := all_skills[skill_name[0]]; ok {
			relative_skill := fmt.Sprintf("Skill: %s (located at .claude/skills/%s) \nPaths in the instructions below are relative to that folder.\n\n", skill_name[0], skill_name[0])
			fmt.Fprintln(os.Stderr, "debug single: ", relative_skill+skill_body)
			messages = append(messages, openai.UserMessage(relative_skill+skill_body))
		} else {
			messages = append(messages, openai.UserMessage(prompt))
		}
	} else {
		messages = append(messages, openai.UserMessage(prompt))
	}

	return messages
}
