// Command data explicitly builds or refreshes persistent reference data. It is
// separate from the web server so normal app usage never triggers providers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/fantasypros"
	"github.com/jrevansjr/ffapp/backend/internal/importer"
)

func main() {
	log.SetFlags(0)
	if err := loadEnvironment(); err != nil {
		log.Fatal(err)
	}
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

// loadEnvironment makes the ignored local .env convenient without overriding
// variables explicitly exported by the caller's shell.
func loadEnvironment() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	runner := importer.NewRunner(database.PathFromEnv(), os.Stdout)
	switch args[0] {
	case "load":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: go run ./cmd/data load teams|players|stats|fantasypros|projections|odds [--refresh]")
		}
		dataset := args[1]
		refresh := len(args) == 3 && args[2] == "--refresh"
		if len(args) == 3 && !refresh {
			return fmt.Errorf("usage: go run ./cmd/data load teams|players|stats|fantasypros|projections|odds [--refresh]")
		}
		if refresh && dataset != "stats" {
			return fmt.Errorf("--refresh is supported only by load stats")
		}
		return runner.Load(ctx, dataset, refresh)
	case "refresh":
		if len(args) != 3 || args[1] != "fantasypros" {
			return fmt.Errorf("usage: go run ./cmd/data refresh fantasypros adp|ecr|projections")
		}
		return runner.RefreshFantasyPros(ctx, fantasypros.DatasetName(args[2]))
	case "build":
		if len(args) != 1 {
			return fmt.Errorf("usage: go run ./cmd/data build")
		}
		return runner.Build(ctx)
	case "rebuild":
		flags := flag.NewFlagSet("rebuild", flag.ContinueOnError)
		confirmed := flags.Bool("confirm", false, "confirm backup and database replacement")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || !*confirmed {
			return fmt.Errorf(
				"rebuild requires --confirm; stop the backend first, then run: go run ./cmd/data rebuild --confirm",
			)
		}
		result, err := runner.Rebuild(ctx)
		if err != nil {
			return err
		}
		if result.BackupPath != "" {
			fmt.Printf("backup: %s\n", result.BackupPath)
		}
		fmt.Printf("database: rebuilt and validated %s\n", result.DBPath)
		return nil
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf(
		"usage: go run ./cmd/data load teams|players|stats|fantasypros|projections|odds [--refresh] | refresh fantasypros adp|ecr|projections | build | rebuild --confirm",
	)
}
