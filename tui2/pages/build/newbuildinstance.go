package build

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/buildoutput"
)

type SetupNewBuildMsg struct{}

func SetupNewBuildCmd() tea.Cmd {
	return func() tea.Msg {
		return SetupNewBuildMsg{}
	}
}

type NewBuildInstance *BuildInstance

type BuildInstance struct {
	Cmd      *exec.Cmd
	BuildOut *buildoutput.BuildOutput
}

func NewBuild(manifestPath string) (*BuildInstance, error) {
	// TODO: could we call the same code that is used in substreams build
	//       instead of starting a sub process?
	cmd := exec.Command("substreams", "build", manifestPath)
	buildOut := buildoutput.New(cmd)

	return &BuildInstance{
		Cmd:      cmd,
		BuildOut: buildOut,
	}, nil
}
