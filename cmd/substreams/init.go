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
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
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
	defaultEndpoint := "https://codegen.substreams.dev"
	if newValue := os.Getenv("SUBSTREAMS_INIT_CODEGEN_ENDPOINT"); newValue != "" {
		defaultEndpoint = newValue
	}
	initCmd.Flags().String("codegen-endpoint", defaultEndpoint, "Endpoint used to discover code generators")
	initCmd.Flags().String("state-file", "./generator.json", "File to load/save the state of the code generator")
	initCmd.Flags().Bool("force-download-cwd", false, "Force download at current dir")
	rootCmd.AddCommand(initCmd)
}

var INIT_TRACE = false
var WITH_ACCESSIBLE = false

type initStateFormat struct {
	GeneratorID string          `json:"generator"`
	State       json.RawMessage `json:"state"`
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
	r, err := regexp.Compile("substreams.*.yaml")
	if err != nil {
		return fmt.Errorf("failed to compile regexp: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to read current directory: %w", err)
	}

	files, err := os.ReadDir(cwd)
	if err != nil {
		return fmt.Errorf("failed to read current directory: %w", err)
	}

	for _, file := range files {
		if r.MatchString(file.Name()) {
			return fmt.Errorf("substreams project already exists in this directory: %q. Try running 'substreams init' in a new directory", file.Name())
		}
	}

	opts := []connect.ClientOption{
		connect.WithGRPC(),
	}

	initConvoURL := sflags.MustGetString(cmd, "codegen-endpoint")
	stateFile, stateFileFlagProvided := sflags.MustGetStringProvided(cmd, "state-file")
	if !strings.HasSuffix(stateFile, ".json") {
		return fmt.Errorf("state file must have a .json extension")
	}

	if stateFileFlagProvided && !cli.FileExists(stateFile) {
		return fmt.Errorf("state file %q does not exist", stateFile)
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

		lastState = state

		if !stateFileFlagProvided {
			useGenerator := true
			inputField := huh.NewConfirm().
				Title(fmt.Sprintf("State file %q was found (%s - %s). Do you want to start from there ?", stateFile, state.GeneratorID, humanize.Time(s.ModTime()))).
				Value(&useGenerator)

			if err := huh.NewForm(huh.NewGroup(inputField)).WithTheme(huh.ThemeCharm()).WithAccessible(WITH_ACCESSIBLE).Run(); err != nil {
				return fmt.Errorf("failed taking confirmation input: %w", err)
			}

			if !useGenerator {
				lastState = &initStateFormat{}
				newName := fmt.Sprintf("%s.%s.json", strings.TrimSuffix(stateFile, ".json"), time.Now().Format("2006-01-02T15-04-05"))
				os.Rename(stateFile, newName)
				fmt.Printf("File %q renamed to %q\n", stateFile, newName)
			}
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

			key := fmt.Sprintf("%-20s - %s", gen.Id, gen.Title)
			if endpoint != "" {
				key = fmt.Sprintf("%-20s (%-40s) - %s", gen.Id, endpoint, gen.Title)
			}

			entry := huh.Option[*pbconvo.DiscoveryResponse_Generator]{
				Key:   key,
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
		Version:     1,
	}
	if lastState.State != nil {
		startMsg.Hydrate = &pbconvo.UserInput_Hydrate{SavedState: string(lastState.State)}
	}

	err = sendFunc(&pbconvo.UserInput{
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

			fmt.Print(ToMarkdown(msgString))

		case *pbconvo.SystemOutput_ImageWithText_:
			input := msg.ImageWithText
			if input.ImgUrl != "" {
				fmt.Println("View image here:", input.ImgUrl)
			}
			if input.Markdown != "" {
				fmt.Println(ToMarkdown(input.Markdown))
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
			selection := msg.ListSelect.DefaultValue
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

			returnValue := msg.Confirm.DefaultButton == pbconvo.SystemOutput_Confirm_CONFIRM
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
			savingDest, _ := os.Getwd()
			input := msg.DownloadFiles

			if len(input.Files) == 0 {
				return fmt.Errorf("no files to download")
			}

			forceDownloadProvided, _ := sflags.MustGetBoolProvided(cmd, "force-download-cwd")
			if forceDownloadProvided {
				for _, inputFile := range input.Files {
					fullpath := path.Join(savingDest, inputFile.Filename)
					fileDir := path.Dir(fullpath)

					err = os.MkdirAll(fileDir, os.ModePerm)
					if err != nil {
						return fmt.Errorf("creating directory %q: %w", fileDir, err)
					}

					err = os.WriteFile(fullpath, inputFile.Content, 0777)
					if err != nil {
						if !os.IsNotExist(err) {
							return fmt.Errorf("writing file %q: %w", fullpath, err)
						}
					}
				}
				return nil
			}

			fmt.Println("Writing local files:")

			// let the terminal breath a little
			// fmt.Println()

			overwriteForm := NewOverwriteForm()

			for _, inputFile := range input.Files {
				if inputFile.Content == nil {
					continue
				}

				fmt.Printf("  - %s\n", filenameStyle(inputFile.Filename))
				if inputFile.Description != "" {
					fmt.Printf("\t%s\n", inputFile.Description)
				}
				switch inputFile.Type {
				case "application/x-zip+extract": // our custom mime type to always extract the file upon arrival
					if inputFile.Content == nil {
						continue
					}

					zipRoot := savingDest

					zipContent := inputFile.Content
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

					fullPath := filepath.Join(savingDest, inputFile.Filename)
					err = saveDownloadFile(fullPath, overwriteForm, inputFile)
					if err != nil {
						return fmt.Errorf("saving file: %w", err)
					}
				}
			}
		default:
			fmt.Printf("Received unknown message type: %T\n", resp.Entry)
		}
	}

	return nil
}

func saveDownloadFile(path string, overwriteForm *OverwriteForm, inputFile *pbconvo.SystemOutput_DownloadFile) (err error) {
	if inputFile.Filename == ".gitignore" {
		if _, err := os.Stat(path); err == nil {
			// Add .gitignore current inputFile content to the existing .gitignore file
			existingContent, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading existing .gitignore file: %w", err)
			}

			// append the new .gitignore content, deduping entries
			existingValues := make(map[string]struct{})
			for _, line := range strings.Split(string(existingContent), "\n") {
				existingValues[line] = struct{}{}
			}

			var appendValues []string
			for _, line := range strings.Split(string(inputFile.Content), "\n") {
				if _, ok := existingValues[line]; !ok {
					appendValues = append(appendValues, line)
				}
			}
			appendBytes := []byte(strings.Join(appendValues, "\n"))

			var out []byte
			if appendValues != nil {
				fmt.Printf("\t-- content appended to existing .gitignore\n\n")
				out = append(existingContent, []byte("\n# Added by substreams init")...)
				out = append(out, appendBytes...)
				out = append(out, []byte("\n")...)
			} else {
				fmt.Printf("\t-- skipped as it contained no new .gitignore entries\n\n")
				out = existingContent
			}

			err = os.WriteFile(path, out, 0644)
			if err != nil {
				return fmt.Errorf("saving merged .gitignore file %q: %w", inputFile.Filename, err)
			}

			return nil
		}

		err = os.WriteFile(path, inputFile.Content, 0644)
		if err != nil {
			return fmt.Errorf("saving .gitignore file %q: %w", inputFile.Filename, err)
		}

		return nil
	}

	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("creating sub-directory %q: %w", path, err)
	}

	if !overwriteForm.OverwriteAll {
		if stat, err := os.Lstat(path); err == nil {
			if stat.Mode().Type() == os.ModeSymlink && inputFile.Filename == "README.md" { // we always overwrite a 'symlink README.md' to match our canonical dev flow
				target, err := os.Readlink(path)
				if err != nil {
					return fmt.Errorf("reading existing symlink %s: %w", path, err)
				}

				lipgloss.NewStyle().Italic(true)
				fmt.Printf("\t-- previous symlink (README.md -> %s) automatically replaced\n", target)
			} else {
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
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("attempting to delete existing file %s: %w", path, err)
	}
	err = os.WriteFile(path, inputFile.Content, 0644)
	if err != nil {
		return fmt.Errorf("saving zip file %q: %w", inputFile.Filename, err)
	}
	return nil
}

func ToMarkdown(input string) string {
	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}

	renderer, err := glamour.NewTermRenderer(glamour.WithWordWrap(0), glamour.WithStandardStyle(style))

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

func filenameStyle(input string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Render(input)
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
			return err
		}
		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, f.Mode())
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
			Key:   "Yes, overwrite all",
			Value: "yes_all",
		},
		{
			Key:   "Yes, overwrite",
			Value: "yes",
		},
		{
			Key:   "No",
			Value: "no",
		},
	}

	var selection string
	selectField := huh.NewSelect[string]().
		Title(fmt.Sprintf("File already exists, do you want to overwrite %s ?", path)).
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
