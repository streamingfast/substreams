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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	connect "connectrpc.com/connect"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
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

		Initialize a new Substreams project using a remote code generator.		
		State will be saved to 'generator.json' by default.

		Example: 
			substreams init
	`),
	RunE:         runSubstreamsInitE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func init() {
	initCmd.Flags().String("discovery-endpoint", "https://codegen.substreams.dev", "Endpoint used to discover code generators")
	initCmd.Flags().String("state-file", "./generator.json", "File to load/save the state of the code generator")
	rootCmd.AddCommand(initCmd)
}

var INIT_TRACE = false
var WITH_ACCESSIBLE = false

type initStateFormat struct {
	GeneratorID string          `json:"generator"`
	State       json.RawMessage `json:"state"`
}

type UserState struct {
	downloadedFilesfolderPath string
}

func newUserState() *UserState {
	return &UserState{}
}

func readGeneratorState(stateFile string) (*initStateFormat, error) {
	stateBytes, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read codegen state file: %w", err)
	}
	var state = &initStateFormat{}
	if err := json.Unmarshal(stateBytes, state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal generator state file: %w", err)
	}
	return state, nil
}

func runSubstreamsInitE(cmd *cobra.Command, args []string) error {
	opts := []connect.ClientOption{
		connect.WithGRPC(),
	}

	initConvoURL := sflags.MustGetString(cmd, "discovery-endpoint")
	stateFile := sflags.MustGetString(cmd, "state-file")
	if !strings.HasSuffix(stateFile, ".json") {
		return fmt.Errorf("state file must have a .json extension")
	}

	transport := &http2.Transport{}
	switch {
	case strings.HasPrefix(initConvoURL, "https://localhost"):
		// don't check certificate on local host
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	case strings.HasPrefix(initConvoURL, "http://"):
		// support for http2 without TLS
		transport.AllowHTTP = true
		transport.DialTLSContext = func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, network, addr)
		}
	}

	client := pbconvoconnect.NewConversationServiceClient(&http.Client{Transport: transport}, initConvoURL, opts...)

	var lastState = &initStateFormat{}
	if s, err := os.Stat(stateFile); err == nil {
		state, err := readGeneratorState(stateFile)
		if err != nil {
			return fmt.Errorf("state file %q file exists, but is invalid: %w", stateFile, err)
		}

		useGenerator := true
		inputField := huh.NewConfirm().
			Title(fmt.Sprintf("State file %q was found (%s - %s). Do you want to start from there ?", stateFile, state.GeneratorID, humanize.Time(s.ModTime()))).
			Value(&useGenerator)

		if err := huh.NewForm(huh.NewGroup(inputField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run(); err != nil {
			return fmt.Errorf("failed taking confirmation input: %w", err)
		}

		if useGenerator {
			lastState = state
		} else {
			newName := fmt.Sprintf("%s.%d.json", strings.TrimSuffix(stateFile, ".json"), time.Now().Unix())
			os.Rename(stateFile, newName)
			fmt.Printf("File %q renamed to %q\n", stateFile, newName)
		}
	}

	generatorID := lastState.GeneratorID
	if generatorID == "" {
		fmt.Printf("Getting available code generators from %s...\n\n", initConvoURL)
		resp, err := client.Discover(context.Background(), connect.NewRequest(&pbconvo.DiscoveryRequest{}))
		if err != nil {
			return fmt.Errorf("failed to call discovery endpoint: %w", err)
		}

		var options []huh.Option[*pbconvo.DiscoveryResponse_Generator]
		for _, gen := range resp.Msg.Generators {
			endpoint := ""
			if gen.Endpoint != "" {
				endpoint = " (" + gen.Endpoint + ")"
			}
			entry := huh.Option[*pbconvo.DiscoveryResponse_Generator]{
				Key:   fmt.Sprintf("%s%s - %s", gen.Id, endpoint, gen.Title),
				Value: gen,
			}
			options = append(options, entry)
		}

		var codegen *pbconvo.DiscoveryResponse_Generator
		selectField := huh.NewSelect[*pbconvo.DiscoveryResponse_Generator]().
			Title("Choose the code generator that you want to use to bootstrap your project").
			Options(options...).
			Value(&codegen)

		err = huh.NewForm(huh.NewGroup(selectField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run()
		if err != nil {
			return fmt.Errorf("failed taking input: %w", err)
		}

		fmt.Println("┃ ", bold("Selected code generator:"), codegen.Id, "-", codegen.Title)
		for i, desc := range strings.Split(codegen.Description, "\n") {
			dd := desc
			if i == 0 {
				dd = bold("Description: ") + desc
			}
			fmt.Println("┃ ", dd)
		}
		lastState.GeneratorID = codegen.Id
		generatorID = codegen.Id
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

	userState := newUserState()

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
			fmt.Printf("Saving state to %q\n", stateFile)
		}

		// TODO: reformat the JSON code into a yaml file or something? Make it editable and readable easily?
		// Nothing fixes the format of the state atm, but we could agree on a format, and fixate JSON or YAML.
		// JSON is probably better for interchange.
		cnt, err := json.MarshalIndent(lastState, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal generator state: %w", err)
		}

		if err = os.WriteFile(stateFile, cnt, 0644); err != nil {
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

			returnValue := input.DefaultValue

			inputField := huh.NewInput().
				Title(input.Prompt).
				Description(input.Description).
				Placeholder(input.Placeholder).
				Value(&returnValue)

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
					}).Run()
				}(loadingCh)

			} else {
				// reset loading channel
				if loadingCh != nil {
					loadingCh <- true
					loadingCh = nil
				}
				fmt.Println(msg.Loading.Label)
			}

		case *pbconvo.SystemOutput_DownloadFiles_:
			if userState.downloadedFilesfolderPath == "" {
				savingDest := "output"
				if projectName := gjson.GetBytes(lastState.State, "name").String(); projectName != "" {
					savingDest = projectName
				}

				if cwd, err := os.Getwd(); err == nil {
					savingDest = filepath.Join(cwd, savingDest)
				}

				inputField := huh.NewInput().Title("In which directory do you want to download the project?").Value(&savingDest)
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

				// the multiple \n are not a mistake, it's to have a blank line before the next message
				fmt.Printf("\nProject will be saved in %s\n", savingDest)
				userState.downloadedFilesfolderPath = savingDest
			}

			input := msg.DownloadFiles
			fmt.Println("Files:")
			for _, file := range input.Files {
				fmt.Printf("  - %s (%s)\n", filepath.Join(userState.downloadedFilesfolderPath, file.Filename), file.Type)
				if file.Description != "" {
					fmt.Println(file.Description)
				}
			}

			// let the terminal breath a little
			fmt.Println()

			if len(input.Files) == 0 {
				return fmt.Errorf("no files to download")
			}

			overwriteForm := NewOverwriteForm()

			for _, inputFile := range input.Files {
				switch inputFile.Type {
				case "application/x-zip+extract": // our custom mime type to always extract the file upon arrival
					if inputFile.Content == nil {
						continue
					}

					zipRoot := userState.downloadedFilesfolderPath

					zipContent := inputFile.Content
					fmt.Printf("Unzipping %s into %s\n", inputFile.Filename, zipRoot)
					err = unzipFile(overwriteForm, zipContent, zipRoot)
					if err != nil {
						return fmt.Errorf("unzipping file: %w", err)
					}

				default:
					// "application/x-protobuf; messageType=\"sf.substreams.v1.Package\""
					// "application/zip", "application/x-zip"
					// "text/plain":
					if inputFile.Content == nil {
						continue
					}

					fullPath := filepath.Join(userState.downloadedFilesfolderPath, inputFile.Filename)
					err = saveDownloadFile(fullPath, overwriteForm, inputFile)
					if err != nil {
						return fmt.Errorf("saving file: %w", err)
					}

				}
			}

			if err := sendFunc(&pbconvo.UserInput{
				FromActionId: resp.ActionId,
				Entry: &pbconvo.UserInput_DownloadedFiles_{
					DownloadedFiles: &pbconvo.UserInput_DownloadedFiles{},
				},
			}); err != nil {
				return fmt.Errorf("error sending confirmation: %w", err)
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

func saveDownloadFile(path string, overwriteForm *OverwriteForm, inputFile *pbconvo.SystemOutput_DownloadFile) (err error) {
	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("creating sub-directory %q: %w", path, err)
	}

	if !overwriteForm.OverwriteAll {
		if _, err := os.Stat(path); err == nil {
			err = overwriteForm.createOverwriteForm(path)
			if err != nil {
				return fmt.Errorf(": %w", err)
			}

			if !overwriteForm.Overwrite {
				fmt.Println("Skipping", path)
				return nil
			}
		}
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
	renderer, err := glamour.NewTermRenderer(glamour.WithWordWrap(160), glamour.WithStandardStyle(style))
	if err != nil {
		panic(fmt.Errorf("failed rendering markdown %q: %w", input, err))
	}
	out, err := renderer.Render(input)
	if err != nil {
		panic(fmt.Errorf("failed rendering markdown %q: %w", input, err))
	}
	return out
}

func bold(input string) string {
	return lipgloss.NewStyle().Bold(true).Render(input)
}

func unzipFile(overwriteForm *OverwriteForm, zipContent []byte, zipRoot string) error {
	reader := bytes.NewReader(zipContent)
	zipReader, err := zip.NewReader(reader, int64(len(zipContent)))
	if err != nil {
		return err
	}

	for _, f := range zipReader.File {
		filePath := filepath.Join(zipRoot, f.Name)

		if !overwriteForm.OverwriteAll {
			if _, err := os.Stat(filePath); err == nil {
				err := overwriteForm.createOverwriteForm(filePath)
				if err != nil {
					return fmt.Errorf(": %w", err)
				}

				if !overwriteForm.Overwrite {
					fmt.Println("Skipping", filePath)
					continue
				}
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

type OverwriteForm struct {
	Overwrite    bool
	OverwriteAll bool
}

func NewOverwriteForm() *OverwriteForm {
	return &OverwriteForm{
		Overwrite:    false,
		OverwriteAll: false,
	}
}

func (f *OverwriteForm) createOverwriteForm(path string) error {
	options := []huh.Option[string]{
		{
			Key:   "No",
			Value: "no",
		},
		{
			Key:   "Yes, overwrite",
			Value: "yes",
		},
		{
			Key:   "Yes, overwrite all",
			Value: "yes_all",
		},
	}

	var selection string
	selectField := huh.NewSelect[string]().
		Title(fmt.Sprintf("File already exists, Do you want to overwrite %s ?", path)).
		Options(options...).
		Value(&selection)

	err := huh.NewForm(huh.NewGroup(selectField)).WithAccessible(WITH_ACCESSIBLE).Run()
	if err != nil {
		f.Overwrite = false
		return fmt.Errorf("failed confirming: %w", err)
	}

	switch selection {
	case "no":
		f.Overwrite = false
	case "yes":
		f.Overwrite = true
	case "yes_all":
		f.Overwrite = true
		f.OverwriteAll = true
	}

	return nil
}
