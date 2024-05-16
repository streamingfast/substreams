package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	connect "connectrpc.com/connect"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	pbconvo "github.com/streamingfast/substreams/pb/sf/codegen/conversation/v1"
	"github.com/streamingfast/substreams/pb/sf/codegen/conversation/v1/pbconvoconnect"
	"golang.org/x/net/http2"
)

var initCmd = &cobra.Command{
	Use:   "init [<path>]",
	Short: "Initialize a new, working Substreams project from scratch",
	Long: cli.Dedent(`
		Initialize a new, working Substreams project from scratch. The path parameter is optional,
		with your current working directory being the default value.

		If you have an Etherscan API Key, you can set it to "ETHERSCAN_API_KEY" environment variable, it will be used to
		fetch the ABIs and contract information.
	`),
	RunE:         runSubstreamsInitE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	initCmd.Flags().String("type", "discover", "ID of the code generator to use. Use 'discover' to list available ones.")

	if x := os.Getenv("ETHERSCAN_API_KEY"); x != "" {
		etherscanAPIKey = x
	}
	rootCmd.AddCommand(initCmd)
}

var INIT_TRACE = false
var WITH_ACCESSIBLE = false

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	opts := []connect.ClientOption{
		connect.WithGRPC(),
	}
	// TODO:  make the `endpoint` here point to `https://codegen.substreams.dev` by default.
	// WARN: when it's not `localhost` in the hostname, don't flip `InsecureSkipVerify` to true!
	httpClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	client := pbconvoconnect.NewConversationServiceClient(httpClient, "https://localhost:9000", opts...)

	topicType := mustGetString(cmd, "type")
	if topicType == "discover" {
		resp, err := client.Discover(context.Background(), connect.NewRequest(&pbconvo.DiscoveryRequest{}))
		if err != nil {
			return fmt.Errorf("failed to call discovery endpoint: %w", err)
		}
		_ = resp.Msg
		fmt.Println("Here is a list of available code generators to help you out:")
		fmt.Println("")
		// TODO: display the discovery, and start the topic from the selected element.
		for idx, topic := range resp.Msg.ConversationTopics {
			fmt.Printf("%d. %s - %s\n%s\n\n", idx+1, bold(topic.Id), topic.Title, topic.Description)
		}
		fmt.Println("Run `substreams init --type` with the desired code generator ID to start a new project.")
		return nil
	}

	var lastState string
	if _, err := os.Stat("substreams.codegen.state"); err == nil {
		fmt.Println("The file 'substreams.codegen.state', was detected, reloading state from that point on.")
		stateBytes, err := os.ReadFile("substreams.codegen.state")
		if err != nil {
			return fmt.Errorf("failed to read codegen state file: %w", err)
		}
		lastState = string(stateBytes)
	} else {
		// TODO: otherwise here, we should ensure that the directory is empty... (or use the specified sub-directory?)
	}

	conn := client.Converse(context.Background())
	sendFunc := func(msg *pbconvo.UserInput) error {
		if INIT_TRACE {
			cnt, _ := json.MarshalIndent(msg.Entry, "", "  ")
			fmt.Printf("OUTPUT: %T %s\n", msg.Entry, string(cnt))
		}
		return conn.Send(msg)
	}
	startMsg := &pbconvo.UserInput_Start{
		TopicId: topicType,
	}
	if lastState != "" {
		// ALSO store in that state file the `conversation` ID and version (ethereum_dynamic_data_source_v1)
		startMsg.Hydrate = &pbconvo.UserInput_Hydrate{SavedState: lastState}
	}
	err := sendFunc(&pbconvo.UserInput{
		Entry: &pbconvo.UserInput_Start_{
			Start: startMsg,
		},
	})
	if err != nil {
		return fmt.Errorf("failed sending start message: %w", err)
	}

	var loadingCh chan bool
	for {
		resp, err := conn.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// TODO: reconnect, and send the Hydrate message to continue the conversation...
			return fmt.Errorf("connection error: %w (state=%q)", err, lastState)
		}
		if resp.State != "" {
			lastState = resp.State
		}
		if INIT_TRACE {
			cnt, _ := json.MarshalIndent(resp.Entry, "", "  ")
			fmt.Printf("INPUT: %T %s\n", resp.Entry, string(cnt))
			fmt.Println("Saving state to substreams.codegen.state")
		}

		// TODO: reformat the JSON code into a yaml file or something? Make it editable and readable easily?
		// Nothing fixes the format of the state atm, but we could agree on a format, and fixate JSON or YAML.
		// JSON is probably better for interchange.
		if err = os.WriteFile("substreams.codegen.state", []byte(lastState), 0644); err != nil {
			return fmt.Errorf("couldn't write codegen state: %w", err)
		}

		switch msg := resp.Entry.(type) {
		case *pbconvo.SystemOutput_Message_:

			msgString := msg.Message.Markdown
			if msg.Message.Style == "error" {
				msgString = "⚠️ " + msg.Message.Markdown
			}
			fmt.Print(toMarkdown(msgString))

		case *pbconvo.SystemOutput_ImageWithText_:
			input := msg.ImageWithText
			if input.ImgUrl != "" {
				fmt.Println("View image here:", input.ImgUrl)
			}
			if input.Markdown != "" {
				fmt.Println(toMarkdown(input.Markdown))
			}

		case *pbconvo.SystemOutput_ListSelect_:
			input := msg.ListSelect

			//fmt.Println(toMarkdown(input.Instructions))

			if len(input.Labels) == 0 && len(input.Values) == 0 {
				fmt.Println("Hmm, the server sent no option to select from (!)")
				continue
			}

			var options []huh.Option[string]
			optionsMap := make(map[string]string)
			for i := 0; i < len(input.Labels); i++ {
				entry := huh.Option[string]{
					Key:   input.Labels[i],
					Value: input.Values[i],
				}
				options = append(options, entry)
				optionsMap[entry.Value] = entry.Key
			}
			var selection string
			selectField := huh.NewSelect[string]().
				Title(input.Instructions).
				Options(options...).
				Value(&selection)

			err := huh.NewForm(huh.NewGroup(selectField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run()
			if err != nil {
				return fmt.Errorf("failed taking input: %w", err)
			}

			fmt.Println("┃ ", input.Instructions)
			for _, opt := range options {
				if opt.Value == selection {
					fmt.Println("┃ -", bold(opt.Key))
				} else {
					fmt.Println("┃ -", opt.Key)
				}
			}

			if err := sendFunc(&pbconvo.UserInput{
				FromActionId: resp.ActionId,
				Entry: &pbconvo.UserInput_Selection_{
					Selection: &pbconvo.UserInput_Selection{
						Label: optionsMap[selection],
						Value: selectField.GetValue().(string),
					},
				},
			}); err != nil {
				return fmt.Errorf("error sending message: %w", err)
			}

		case *pbconvo.SystemOutput_TextInput_:
			input := msg.TextInput

			var returnValue string
			inputField := huh.NewInput().
				Title(input.Prompt).
				Description(input.Description).
				Placeholder(input.Placeholder).
				Value(&returnValue)
			if input.DefaultValue != "" {
				inputField.Suggestions([]string{input.DefaultValue})
			}

			if input.ValidationRegexp != "" {
				validationRE, err := regexp.Compile(input.ValidationRegexp)
				if err != nil {
					return fmt.Errorf("invalid regexp received from server (%q) to validate text input: %w", msg.TextInput.ValidationRegexp, err)
				}
				inputField.Validate(func(userInput string) error {
					matched := validationRE.MatchString(userInput)
					if !matched {
						return errors.New(input.ValidationErrorMessage)
					}
					return nil
				})
			}

			err := huh.NewForm(huh.NewGroup(inputField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run()
			if err != nil {
				return fmt.Errorf("failed taking input: %w", err)
			}

			fmt.Println("┃ ", input.Prompt+":", bold(returnValue))

			if err := sendFunc(&pbconvo.UserInput{
				FromActionId: resp.ActionId,
				Entry: &pbconvo.UserInput_TextInput_{
					TextInput: &pbconvo.UserInput_TextInput{Value: returnValue},
				},
			}); err != nil {
				return fmt.Errorf("error sending message: %w", err)
			}

		case *pbconvo.SystemOutput_Confirm_:
			input := msg.Confirm

			var returnValue bool
			inputField := huh.NewConfirm().
				Title(input.Prompt).
				Affirmative(input.AcceptButtonLabel).
				Negative(input.DeclineButtonLabel).
				Description(input.Description).
				Value(&returnValue)

			err := huh.NewForm(huh.NewGroup(inputField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run()
			if err != nil {
				return fmt.Errorf("failed taking confirmation input: %w", err)
			}

			affirm := input.AcceptButtonLabel
			deny := input.DeclineButtonLabel
			if returnValue {
				affirm = bold(affirm)
			} else {
				deny = bold(deny)
			}
			fmt.Println("┃ ", input.Prompt)
			fmt.Println("┃ ")
			fmt.Println("┃  " + affirm + "     " + deny)

			if err := sendFunc(&pbconvo.UserInput{
				FromActionId: resp.ActionId,
				Entry: &pbconvo.UserInput_Confirmation_{
					Confirmation: &pbconvo.UserInput_Confirmation{
						Affirmative: returnValue,
					},
				},
			}); err != nil {
				return fmt.Errorf("error sending confirmation: %w", err)
			}

		case *pbconvo.SystemOutput_Loading_:
			input := msg.Loading
			if input.Loading {
				if loadingCh != nil {
					loadingCh <- false
				}
				loadingCh = make(chan bool)
				go func(loadingCh chan bool) {
					_ = spinner.New().Title(msg.Loading.Label).Action(func() {
						<-loadingCh
					}).Run()
				}(loadingCh)
			} else {
				if loadingCh != nil {
					loadingCh <- true
					loadingCh = nil
				}
				fmt.Println(msg.Loading.Label)
			}

		case *pbconvo.SystemOutput_DownloadFiles_:
			input := msg.DownloadFiles
			fmt.Println("Files:")
			for _, file := range input.Files {
				fmt.Printf("  - %s (%s)\n", file.Filename, file.Type)
				fmt.Println(toMarkdown(file.Description))
			}

			spkgRoot := filepath.Base(input.Files[0].Filename)
			spkgRoot = strings.TrimSuffix(spkgRoot, filepath.Ext(spkgRoot))
			spkgRoot = strings.TrimSuffix(spkgRoot, ".")

			var unpackSource bool
			confirm := huh.NewConfirm().
				Title("Unzip source code? ").
				Description(toMarkdown("Will unpack in **" + spkgRoot + "**")).
				Affirmative("Yes, unzip sources").
				Negative("No, I just want the .spkg").
				Inline(true).
				Value(&unpackSource)

			err := huh.NewForm(huh.NewGroup(confirm)).WithAccessible(WITH_ACCESSIBLE).Run()
			if err != nil {
				return fmt.Errorf("failed confirming: %w", err)
			}

			// TODO: offer to write to a different place, or add ` (1)` at the end of the filename
			// if there's conflict.
			if unpackSource {
				// TODO: unzip the input.Files[1].Content into the `spkgRoot` directory.
				fmt.Println("Unzipping", input.Files[1].Filename, "into ./"+spkgRoot)
				fmt.Println("TODO...")
			}

			// TODO: shouldn't this be controlled by the remote end? Maybe there's some follow-up messages,
			// maybe we'll be building three modules in a swift?
			fmt.Println("")
			fmt.Println("Everything done!")

			return nil

		default:
			fmt.Printf("Received unknown message type: %T\n", resp.Entry)
		}
	}

	return nil
}

type initListElement struct {
	Label string
	Value string
}

func toMarkdown(input string) string {
	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}
	out, err := glamour.Render(input, style)
	if err != nil {
		panic(fmt.Errorf("failed rendering markdown %q: %w", input, err))
	}
	return out
}

func bold(input string) string {
	return lipgloss.NewStyle().Bold(true).Render(input)
}
