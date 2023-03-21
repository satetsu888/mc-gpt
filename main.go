package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/otiai10/openaigo"
)

type requestBody struct {
	PlayerName string `json:"player_name"`
	Message    string `json:"message"`
}

type Pos struct {
	X int
	Y int
	Z int
}

var systemContent = `
Assistant is a great Minecraft builder and knowledgeable about Minecraft commands.

Assistant can respond some commands and a description message.
commands is valid minecraft command.
Assistant response example format is always as follows:

` + "```" + `
/setblock ~ ~ ~ minecraft:stone
/fill ~ ~ ~ ~ ~ ~ minecraft:stone
` + "```" + `

Place a stone block at your feet.
`

func userContent(playerPosition Pos, facing string, message string) string {
	return `
ユーザの現在位置は ` + fmt.Sprintf("(%d, %d, %d)", playerPosition.X, playerPosition.Y, playerPosition.Z) + `で` + facing + `を向いています。
以下の建造物を作るコマンドを教えてください

	` + message
}

func buildOpenAIRequest(message string) openaigo.ChatCompletionRequestBody {
	pos := Pos{X: -102, Y: 124, Z: -19}

	return openaigo.ChatCompletionRequestBody{
		Model: "gpt-3.5-turbo",
		Messages: []openaigo.ChatMessage{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userContent(pos, "north", message)},
		},
	}
}

func parseHTTPReq(r *http.Request) (*requestBody, error) {
	var requestBody requestBody

	length, err := strconv.Atoi(r.Header.Get("Content-Length"))
	if err != nil {
		return nil, err
	}

	body := make([]byte, length)

	length, err = r.Body.Read(body)
	if err != nil && err != io.EOF {
		return nil, err
	}

	err = json.Unmarshal(body[:length], &requestBody)
	if err != nil {
		return nil, err
	}

	return &requestBody, nil
}

func parseOpenAIResponse(response openaigo.ChatCompletionResponse) (string, string) {
	content := response.Choices[0].Message.Content
	commandRegexp := regexp.MustCompile("(?s)```(.+?)```")
	commandResults := commandRegexp.FindAllStringSubmatch(content, -1)

	commands := []string{}
	for _, commandResult := range commandResults {
		for _, line := range strings.Split(commandResult[1], "\n") {
			if line == "" {
				continue
			}
			commands = append(commands, line)
		}
	}

	descriptionRegexp := regexp.MustCompile("(?s)```([^`]+?)\\z")
	descriptionResult := descriptionRegexp.FindStringSubmatch(content)

	return strings.Join(commands, "\n"), descriptionResult[1]
}

func buildHandler(ctx context.Context) http.HandlerFunc {
	client := openaigo.NewClient(os.Getenv("OPENAI_API_KEY"))

	return func(w http.ResponseWriter, r *http.Request) {
		requestBody, err := parseHTTPReq(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		request := buildOpenAIRequest(requestBody.Message)
		response, err := client.Chat(ctx, request)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println(err)
			return
		}
		fmt.Printf("all: %v ¥n¥n ", response)

		mcCode, description := parseOpenAIResponse(response)

		fmt.Println(mcCode)
		fmt.Println(description)
		fmt.Fprintf(w, "%v", description)
	}
}

func main() {
	ctx := context.Background()

	http.HandleFunc("/", buildHandler(ctx))
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		fmt.Println(err)
	}
}
