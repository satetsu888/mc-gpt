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
	rconClient "github.com/satetsu888/minecraft-rcon-builder/client"
	"github.com/satetsu888/minecraft-rcon-builder/model"
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
The assistant possesses great Minecraft building skills and extensive knowledge of Minecraft commands.
The player is currently playing Minecraft Java Edition.
In Minecraft, the negative X-axis corresponds to facing west, while the positive X-axis corresponds to facing east.
Similarly, facing north corresponds to the negative Z-axis, and facing south corresponds to the positive Z-axis.
Position is specified as X, Y, Z order.

The assistant is capable of responding to certain commands and providing a description message.
These commands will execute in the Minecraft world with operator privileges.
The assistant's response follows a specific format, as demonstrated below:

` + "```" + `
/setblock 100 64 120 minecraft:oak_planks
/fill 110 64 130 150 67 170 minecraft:oak_planks
` + "```" + `

To place some oak planks blocks.
`

func userContent(playerPosition Pos, facing string, message string) string {
	return `
ユーザの現在位置は ` + fmt.Sprintf("(X: %d, Y: %d, Z: %d)", playerPosition.X, playerPosition.Y, playerPosition.Z) + `で` + facing + `を向いています。
以下の内容を実行するコマンドを教えてください

	` + message
}

func buildOpenAIRequest(pos Pos, facing string, message string) openaigo.ChatCompletionRequestBody {
	return openaigo.ChatCompletionRequestBody{
		Model: "gpt-3.5-turbo",
		Messages: []openaigo.ChatMessage{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userContent(pos, facing, message)},
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

	description := ""
	if len(descriptionResult) < 1 {
		description = ""
	} else {
		description = descriptionResult[1]
	}

	return strings.Join(commands, "\n"), description
}

func buildHandler(ctx context.Context) http.HandlerFunc {

	rconHostPort := os.Getenv("RCON_HOSTPORT")
	rconPassowrd := os.Getenv("RCON_PASSWORD")

	rconClient, err := rconClient.NewClient(rconHostPort, rconPassowrd)
	if err != nil {
		panic(err)
	}

	openAIClient := openaigo.NewClient(os.Getenv("OPENAI_API_KEY"))

	return func(w http.ResponseWriter, r *http.Request) {
		requestBody, err := parseHTTPReq(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _, list, err := rconClient.FetchPlayerList()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println(err)
			return
		}

		player := model.Player{}

		for i, playerName := range list {
			if playerName == requestBody.PlayerName {
				fmt.Printf("player found: %v\n", playerName)
				fetchedPlayer, err := rconClient.FetchPlayer(list[i])
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Println(err)
					return
				}
				player = fetchedPlayer
				fmt.Println(player)
				break
			}
		}

		if player.Name == "" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "player not found: %v", requestBody.PlayerName)
			return
		}

		pos := Pos{player.Position().X, player.Position().Y, player.Position().Z}
		fmt.Println(pos)
		facing := string(player.Direction())
		fmt.Println(facing)

		request := buildOpenAIRequest(pos, facing, requestBody.Message)
		response, err := openAIClient.Chat(ctx, request)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println(err)
			return
		}

		mcCode, description := parseOpenAIResponse(response)

		for _, command := range strings.Split(mcCode, "\n") {
			cmd := ""
			if len(command) > 0 && command[0:1] == "/" {
				cmd = command[1:]
			} else {
				cmd = command
			}
			executeCmd := fmt.Sprintf("execute at %s as %s run %s", player.Name, player.Name, cmd)
			fmt.Println("sending command: " + executeCmd)
			msg, err := rconClient.Client.SendCommand(executeCmd)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Println(err)
				return
			}
			fmt.Println(msg)
		}

		fmt.Println(description)
		fmt.Fprintf(w, "%v", response)
	}
}

func main() {
	ctx := context.Background()

	http.HandleFunc("/", buildHandler(ctx))
	fmt.Println("start server")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		fmt.Println(err)
	}
}
