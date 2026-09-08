package devenv

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
)

// Cmd boots the stack and blocks. It is registered under `substreams tools`.
var Cmd = &cobra.Command{
	Use:   "devenv",
	Short: "Run a local Substreams stack against a dummy blockchain",
	Long: `Boots a dummy blockchain, a tier1 and a tier2 built from this source tree, prints the
tier1 endpoint and stays up until interrupted. Requires a working Docker daemon.

A large genesis burst against a small state bundle size gives thousands of segments to chew
through, which is what makes backprocessing long enough to observe. The state store lives
under --data-dir, so a second run of the same request finds everything cached and returns
instantly; delete that directory to get a cold backprocess back.

The stack logs every block it reads at info level, which buries the endpoint banner. Set
DLOG=warn to quiet it down, or DLOG=debug when the stack itself is what misbehaves.`,
	RunE:         runDevenv,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
}

func init() {
	Cmd.Flags().String("data-dir", "", "Directory holding blocks and substreams state, a temporary one is created when empty")
	Cmd.Flags().Int("burst", 20_000, "Number of blocks produced immediately at genesis, this is what there is to backprocess. The merger bundles them at roughly a hundred blocks a second and the stack is only ready once it has caught up, so a larger burst is a proportionally longer wait")
	Cmd.Flags().Int("block-rate", 120, "Blocks per minute produced after the genesis burst")
	Cmd.Flags().Uint64("bundle-size", 100, "Segment size, smaller means more and shorter parallel jobs")
	Cmd.Flags().Uint64("max-subrequests", 10, "How many tier2 jobs may run at once")
	Cmd.Flags().String("image", DefaultImage, "Dummy blockchain image")
	Cmd.Flags().String("reader-args", "", "Extra arguments passed to the reader node")
}

func runDevenv(cmd *cobra.Command, _ []string) error {
	// The root command runs with context.Background(), so nothing here would ever observe a
	// Ctrl-C: the wait below would block forever and the process would be killed outright,
	// taking the deferred teardown with it and leaving the container running.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	burst := sflags.MustGetInt(cmd, "burst")

	dataDir, err := resolveDataDir(sflags.MustGetString(cmd, "data-dir"))
	if err != nil {
		return err
	}

	fmt.Printf("Data directory: %s\n", dataDir)
	fmt.Printf("Starting dummy blockchain (%d blocks at genesis)…\n", burst)

	container, err := StartDummyBlockchain(ctx, ChainConfig{
		Image:           sflags.MustGetString(cmd, "image"),
		TmpDir:          dataDir,
		Burst:           burst,
		BlockRate:       sflags.MustGetInt(cmd, "block-rate"),
		ExtraReaderArgs: sflags.MustGetString(cmd, "reader-args"),
	})
	if err != nil {
		return fmt.Errorf("start dummy blockchain: %w", err)
	}
	defer func() {
		// Ctrl-C is the normal way out of this command, so by the time this runs ctx is
		// cancelled — and a cancelled context makes the Docker call fail on the spot, leaving
		// the container running. cmd.Root().Context() is no help: cobra hands the child the very
		// same context object. Detach from the cancellation to get the teardown through, and
		// bound it so a wedged daemon cannot hang the exit instead.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()

		if err := container.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed terminating container: %s\n", err)
		}
	}()

	relayerEndpoint, err := RelayerEndpoint(ctx, container)
	if err != nil {
		return err
	}

	// Tier1 cannot become ready until the merger has caught up with the burst, see
	// WaitMergedBlocks.
	fmt.Printf("Waiting for the merger to bundle %d blocks…\n", burst)
	if err := WaitMergedBlocks(ctx, dataDir, uint64(burst), mergeTimeoutFor(burst)); err != nil {
		return fmt.Errorf("merger never caught up: %w", err)
	}

	fmt.Println("Starting tier2…")
	_, tier2Endpoint, err := StartTier2(ctx, Tier2Config{TmpDir: dataDir}, zlog)
	if err != nil {
		return fmt.Errorf("start tier2: %w", err)
	}

	fmt.Println("Starting tier1…")
	_, tier1Endpoint, err := StartTier1(ctx, Tier1Config{
		TmpDir:          dataDir,
		RelayerEndpoint: relayerEndpoint,
		Tier2Endpoint:   tier2Endpoint,
		StateBundleSize: sflags.MustGetUint64(cmd, "bundle-size"),
		MaxSubrequests:  sflags.MustGetUint64(cmd, "max-subrequests"),
		MetricsPrefix:   "devenv",
	}, zlog)
	if err != nil {
		return fmt.Errorf("start tier1: %w", err)
	}

	fmt.Printf(`
Stack ready.

  tier1     %s
  tier2     %s
  relayer   %s

Watch a cold backprocess, against a substreams with a store:

  substreams run -e %s --plaintext \
    ./tests_e2e/partial_blocks_store/partial-blocks-store-v0.1.0.spkg map_tx_counter_summary \
    -s %d --production-mode --limit-processed-blocks 0

Ctrl-C to tear everything down.
`, tier1Endpoint, tier2Endpoint, relayerEndpoint, tier1Endpoint, burst*3/4)

	<-ctx.Done()
	fmt.Println("\nShutting down…")

	return nil
}

// mergeTimeoutFor allows well over the observed merge rate of roughly a hundred blocks a
// second, so that a slow machine reports the real problem rather than a spurious timeout.
func mergeTimeoutFor(burst int) time.Duration {
	return time.Minute + time.Duration(burst/50)*time.Second
}

func resolveDataDir(dataDir string) (string, error) {
	var err error
	if dataDir == "" {
		if dataDir, err = os.MkdirTemp("", "substreams-devenv-"); err != nil {
			return "", fmt.Errorf("temp dir: %w", err)
		}
	}

	if dataDir, err = filepath.Abs(dataDir); err != nil {
		return "", fmt.Errorf("absolute data dir: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}

	return dataDir, nil
}
