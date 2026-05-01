package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	. "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
)

var createUserCmd = Command(createUserE,
	"create-user <dsn> <username> <database>",
	"Create a user in the database",
	ExactArgs(3),
	Flags(func(flags *pflag.FlagSet) {
		AddCommonDatabaseChangesFlags(flags)

		flags.Int("retries", 3, "Number of retries to attempt when a connection error occurs")
		flags.Bool("read-only", false, "Create a read-only user")
		flags.String("password-env", "", "Name of the environment variable containing the password")
	}),
)

func createUserE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	dsnString := args[0]
	username := args[1]
	database := args[2]

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

	dsn, err := db2.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parsing dsn: %w", err)
	}

	if err := retry(ctx, func(ctx context.Context) error {
		handleReorgs := false
		dbLoader, err := db2.NewLoader(
			dsn,
			cursorTableName,
			historyTableName,
			sflags.MustGetString(cmd, "clickhouse-cluster"),
			0, 0, 0,
			db2.OnModuleHashMismatchError.String(),
			&handleReorgs,
			zlog, tracer,
		)

		err = dbLoader.CreateUser(ctx, username, password, database, readOnly)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		return nil
	}, sflags.MustGetInt(cmd, "retries")); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

func retry(ctx context.Context, f func(ctx context.Context) error, reties int) error {
	var err error

	for i := 0; i < reties; i++ {
		err = f(ctx)
		if err == nil {
			return nil
		}
		time.Sleep(5*time.Duration(i)*time.Second + 1*time.Second)
	}

	return err
}
