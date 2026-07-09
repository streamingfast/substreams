package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
)

var sinkSQLCreateUserCmd = &cobra.Command{
	Use:   "create-user <username> <database>",
	Short: "Create a user in the database",
	Args:  cobra.ExactArgs(2),
	RunE:  sinkSQLCreateUserE,
}

func init() {
	flags := sinkSQLCreateUserCmd.Flags()
	addCommonDatabaseChangesFlags(flags)

	flags.Int("retries", 3, "Number of retries to attempt when a connection error occurs")
	flags.Bool("read-only", false, "Create a read-only user")
	flags.String("password-env", "", "Name of the environment variable containing the password")

	sinkSQLCmd.AddCommand(sinkSQLCreateUserCmd)
}

func sinkSQLCreateUserE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	ctx := cmd.Context()

	dsnString, err := resolveSinkSQLDSN(cmd)
	if err != nil {
		return err
	}

	username := args[0]
	database := args[1]

	cursorTableName := sflags.MustGetString(cmd, "cursors-table")
	historyTableName := sflags.MustGetString(cmd, "history-table")

	readOnly := sflags.MustGetBool(cmd, "read-only")
	passwordEnv := sflags.MustGetString(cmd, "password-env")

	if passwordEnv == "" {
		return fmt.Errorf("password-env is required")
	}

	password := os.Getenv(passwordEnv)
	if password == "" {
		return fmt.Errorf("non-empty password is required")
	}

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parsing dsn: %w", err)
	}

	if err := sinkSQLRetry(ctx, func(ctx context.Context) error {
		handleReorgs := false
		dbLoader, err := db.NewLoader(
			dsn,
			cursorTableName,
			historyTableName,
			sflags.MustGetString(cmd, "clickhouse-cluster"),
			0, 0, 0,
			db.OnModuleHashMismatchError.String(),
			&handleReorgs,
			zlog, tracer,
		)
		if err != nil {
			return fmt.Errorf("creating loader: %w", err)
		}

		if err := dbLoader.CreateUser(ctx, username, password, database, readOnly); err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		return nil
	}, sflags.MustGetInt(cmd, "retries")); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

func sinkSQLRetry(ctx context.Context, f func(ctx context.Context) error, retries int) error {
	var err error
	for i := 0; i < retries; i++ {
		err = f(ctx)
		if err == nil {
			return nil
		}
		time.Sleep(5*time.Duration(i)*time.Second + 1*time.Second)
	}
	return err
}
