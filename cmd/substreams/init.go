package main

import (
	"archive/zip"
	"bytes"
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
	"github.com/streamingfast/cli/sflags"
	pbconvo "github.com/streamingfast/substreams/pb/sf/codegen/conversation/v1"
	"github.com/streamingfast/substreams/pb/sf/codegen/conversation/v1/pbconvoconnect"
	"github.com/tidwall/gjson"
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

		Example: 
			substreams init --generator ethereum_init_v1
	`),
	RunE:         runSubstreamsInitE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	initCmd.Flags().String("generator", "discover", "Identifier of the code generator to use. Use 'discover' to list available ones.")
	initCmd.Flags().Bool("local-dev", false, "Run the generator in local development mode.")

	if x := os.Getenv("ETHERSCAN_API_KEY"); x != "" {
		etherscanAPIKey = x
	}
	rootCmd.AddCommand(initCmd)
}

var INIT_TRACE = false
var WITH_ACCESSIBLE = false

type initStateFormat struct {
	GeneratorID string          `json:"generator"`
	State       json.RawMessage `json:"state"`
}

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	opts := []connect.ClientOption{
		connect.WithGRPC(),
	}

	// TODO:  make the `endpoint` here point to `https://codegen.substreams.dev` by default.
	// WARN: when it's not `localhost` in the hostname, don't flip `InsecureSkipVerify` to true!

	localDev := sflags.MustGetBool(cmd, "local-dev")
	initConvoURL := "https://codegen.substreams.dev"

	if localDev {
		initConvoURL = "https://localhost:9001"
	}

	httpClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	client := pbconvoconnect.NewConversationServiceClient(httpClient, initConvoURL, opts...)

	var lastState = initStateFormat{}
	if _, err := os.Stat("generator.json"); err == nil {
		fmt.Println("The file 'generator.json', was detected, reloading state from that point on.")
		stateBytes, err := os.ReadFile("generator.json")
		if err != nil {
			return fmt.Errorf("failed to read codegen state file: %w", err)
		}
		if err := json.Unmarshal(stateBytes, &lastState); err != nil {
			return fmt.Errorf("failed to unmarshal generator state file: %w", err)
		}
	} else {
		// TODO: otherwise here, we should ensure that the directory is empty... (or use the specified sub-directory?)
	}

	//TODO: the way it is handled right now, this should not be a flag, but a required argument
	generatorID := sflags.MustGetString(cmd, "generator")

	if lastState.GeneratorID != "" && generatorID != "discover" && generatorID != lastState.GeneratorID {
		fmt.Println("Mismatch between the generator ID in `generator.json` and the one specified on the command line.")
		return fmt.Errorf("generator ID mismatch: %q != %q", generatorID, lastState.GeneratorID)
	}

	if generatorID == "discover" {
		resp, err := client.Discover(context.Background(), connect.NewRequest(&pbconvo.DiscoveryRequest{}))
		if err != nil {
			return fmt.Errorf("failed to call discovery endpoint: %w", err)
		}
		_ = resp.Msg
		fmt.Println("Here is a list of available code generators to help you out:")
		fmt.Println("")
		// TODO: display the discovery, and start the topic from the selected element.
		for idx, generator := range resp.Msg.Generators {
			fmt.Printf("%d. %s - %s\n%s\n\n", idx+1, bold(generator.Id), generator.Title, generator.Description)
		}
		fmt.Println("Run `substreams init --generator` with the desired code generator ID to start a new project.")
		return nil
	} else {
		if lastState.GeneratorID == "" {
			lastState.GeneratorID = generatorID
		}
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
		GeneratorId: generatorID,
	}
	if lastState.State != nil {
		startMsg.Hydrate = &pbconvo.UserInput_Hydrate{SavedState: string(lastState.State)}
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
			return fmt.Errorf("connection error: %w", err)
		}
		if resp.State != "" {
			lastState.State = []byte(resp.State)
		}
		if INIT_TRACE {
			cnt, _ := json.MarshalIndent(resp.Entry, "", "  ")
			fmt.Printf("INPUT: %T %s\n", resp.Entry, string(cnt))
			fmt.Println("Saving state to generator.json")
		}

		// TODO: reformat the JSON code into a yaml file or something? Make it editable and readable easily?
		// Nothing fixes the format of the state atm, but we could agree on a format, and fixate JSON or YAML.
		// JSON is probably better for interchange.
		cnt, err := json.MarshalIndent(lastState, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal generator state: %w", err)
		}
		if err = os.WriteFile("generator.json", cnt, 0644); err != nil {
			return fmt.Errorf("error writing generator state %w", err)
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
				// TODO: notify the server? stop the process?!
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
			fmt.Println("")

			if err := sendFunc(&pbconvo.UserInput{
				FromActionId: resp.ActionId,
				Entry: &pbconvo.UserInput_Selection_{
					Selection: &pbconvo.UserInput_Selection{
						Label: optionsMap[selection],
						Value: selection,
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
					matched := validationRE.MatchString(strings.TrimRight(returnValue, " "))
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
			fmt.Println("")

			if err := sendFunc(&pbconvo.UserInput{
				FromActionId: resp.ActionId,
				Entry: &pbconvo.UserInput_TextInput_{
					TextInput: &pbconvo.UserInput_TextInput{Value: strings.TrimRight(returnValue, " ")},
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
						fmt.Println(msg.Loading.Label)
					}).Run()
				}(loadingCh)

			} else {
				if loadingCh != nil {
					loadingCh <- true
					loadingCh = nil
					fmt.Println(msg.Loading.Label)
				}
			}

		case *pbconvo.SystemOutput_DownloadFiles_:
			input := msg.DownloadFiles
			fmt.Println("Files:")
			for _, file := range input.Files {
				fmt.Printf("  - %s (%s)\n", file.Filename, file.Type)
				if file.Description != "" {
					fmt.Println(file.Description)
				}
			}

			// let the terminal breath a little
			fmt.Println()

			if len(input.Files) == 0 {
				return fmt.Errorf("no files to download")
			}

			sendDownloadedFilesConfirmation := false
			downloadedFilesfolderPath := ""
			for _, inputFile := range input.Files {
				switch inputFile.Type {
				case "application/x-zip":

					savingDest := "output"
					if projectName := gjson.GetBytes(lastState.State, "name").String(); projectName != "" {
						savingDest = projectName
					}
					if cwd, err := os.Getwd(); err == nil {
						savingDest = filepath.Join(cwd, savingDest)
					}
					inputField := huh.NewInput().Title("In which directory do you want to store your source code?").Value(&savingDest)

					inputField.Validate(func(userInput string) error {
						fmt.Println("Checking directory", userInput)
						fileInfo, err := os.Stat(userInput)
						if err != nil {
							if os.IsNotExist(err) {
								return nil
							}
							return fmt.Errorf("error checking directory: %w", err)
						}

						if !fileInfo.IsDir() {
							return errors.New("the path is not a directory")
						}

						return nil
					})

					err := huh.NewForm(huh.NewGroup(inputField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run()
					if err != nil {
						return fmt.Errorf("failed taking input: %w", err)
					}

					zipRoot := savingDest
					downloadedFilesfolderPath = zipRoot

					// the multiple \n are not a mistake, it's to have a blank line before the next message
					fmt.Printf("\nSource code will be saved in %s\n", zipRoot)

					var unpackSource bool
					confirm := huh.NewConfirm().
						Title("Unzip source code? ").
						Affirmative("Yes, unzip sources").
						Negative("No").
						Value(&unpackSource)

					err = huh.NewForm(huh.NewGroup(confirm)).WithAccessible(WITH_ACCESSIBLE).Run()
					if err != nil {
						return fmt.Errorf("failed confirming: %w", err)
					}

					sourcePath := filepath.Join(zipRoot, inputFile.Filename)
					err = saveDownloadFile(sourcePath, inputFile)
					if err != nil {
						return fmt.Errorf("saving zip file: %w", err)
					}

					// if there's conflict.
					if unpackSource {
						zipContent := inputFile.Content
						fmt.Printf("Unzipping %s into %s\n", inputFile.Filename, zipRoot)
						err := unzipFile(zipContent, zipRoot)
						if err != nil {
							return fmt.Errorf("unzipping file: %w", err)
						}
					}

					sendDownloadedFilesConfirmation = true

				case "application/x-protobuf; messageType=\"sf.substreams.v1.Package\"":
					filePath := inputFile.Filename

					if _, err := os.Stat(inputFile.Filename); err == nil {
						overwrite, err := creatingOverwriteForm(filePath)
						if err != nil {
							return fmt.Errorf(": %w", err)
						}

						if !overwrite {
							fmt.Println("Skipping", filePath)
							continue
						}
					}

					err = saveDownloadFile(filePath, inputFile)
					if err != nil {
						return fmt.Errorf("saving spkg file: %w", err)
					}

				case "text/plain":
					fmt.Println("Compilation Logs:")
					fmt.Println(string(inputFile.Content))

				case "text/plain; option:\"save\"":
					err := os.WriteFile(inputFile.Filename, inputFile.Content, 0644)
					if err != nil {
						return fmt.Errorf("saving file: %w", err)
					}

				default:
					fmt.Println("Unknown file type:", inputFile.Type)
				}
			}

			// only need to send a confirmation when not downloading spkg files
			if sendDownloadedFilesConfirmation {
				if err := sendFunc(&pbconvo.UserInput{
					FromActionId: resp.ActionId,
					Entry: &pbconvo.UserInput_DownloadedFiles_{
						DownloadedFiles: &pbconvo.UserInput_DownloadedFiles{
							FolderPath: downloadedFilesfolderPath,
						},
					},
				}); err != nil {
					return fmt.Errorf("error sending confirmation: %w", err)
				}
			}

		default:
			fmt.Printf("Received unknown message type: %T\n", resp.Entry)
		}
	}

	// TODO: shouldn't this be controlled by the remote end? Maybe there's some follow-up messages,
	// maybe we'll be building three modules in a swift?

	fmt.Println("Everything done!")
	return nil
}

type initListElement struct {
	Label string
	Value string
}

func saveDownloadFile(path string, inputFile *pbconvo.SystemOutput_DownloadFile) (err error) {
	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("creating sub-directory %q: %w", path, err)
	}

	overwrite := true
	if _, err := os.Stat(path); err == nil {
		overwrite, err = creatingOverwriteForm(path)
		if err != nil {
			return fmt.Errorf(": %w", err)
		}
	}

	if !overwrite {
		fmt.Println("Skipping", path)
		return nil
	}

	err = os.WriteFile(path, inputFile.Content, 0644)
	if err != nil {
		return fmt.Errorf("saving zip file %q: %w", inputFile.Filename, err)
	}
	return nil
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

func unzipFile(zipContent []byte, zipRoot string) error {
	reader := bytes.NewReader(zipContent)
	zipReader, err := zip.NewReader(reader, int64(len(zipContent)))
	if err != nil {
		return err
	}

	for _, f := range zipReader.File {
		filePath := filepath.Join(zipRoot, f.Name)

		if _, err := os.Stat(filePath); err == nil {
			overwrite, err := creatingOverwriteForm(filePath)
			if err != nil {
				return fmt.Errorf(": %w", err)
			}

			if !overwrite {
				fmt.Println("Skipping", filePath)
				continue
			}
		}

		srcFile, err := f.Open()
		if err != nil {
			return err
		}
		defer srcFile.Close()

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return (err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return (err)
		}
		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		defer destFile.Close()

		if _, err = io.Copy(destFile, srcFile); err != nil {
			return err
		}
	}
	return nil
}

func creatingOverwriteForm(path string) (bool, error) {
	var overwrite bool
	confirm := huh.NewConfirm().
		Title(fmt.Sprintf("File already exists, Do you want to overwrite %s ?", path)).
		Affirmative("Yes, overwrite").
		Negative("No").
		Value(&overwrite)

	err := huh.NewForm(huh.NewGroup(confirm)).WithAccessible(WITH_ACCESSIBLE).Run()
	if err != nil {
		return false, fmt.Errorf("failed confirming: %w", err)
	}

	return overwrite, nil
}
