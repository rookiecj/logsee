package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"logsee/internal/adapter/app"
	"logsee/internal/adapter/cli"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	return runWithDeps(args, os.Stdin, stdout, stderr, defaultCommandDeps())
}

type runAppFunc func(context.Context, cli.Options, io.Reader, io.Writer, app.RunOptions) error

type commandDeps struct {
	openTTY func() (io.ReadCloser, error)
	runApp  runAppFunc
}

func defaultCommandDeps() commandDeps {
	return commandDeps{
		openTTY: func() (io.ReadCloser, error) {
			return os.Open("/dev/tty")
		},
		runApp: app.Run,
	}
}

func runWithDeps(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, deps commandDeps) int {
	if stdin == nil {
		stdin = os.Stdin
	}
	if deps.runApp == nil {
		deps.runApp = app.Run
	}

	options, err := cli.ParseArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, cli.Usage())
			return 0
		}
		fmt.Fprintln(stderr, err)
		return 2
	}
	if options.Version {
		fmt.Fprintf(stdout, "logsee %s\n", version)
		return 0
	}

	runOptions := app.RunOptions{
		Interactive:  true,
		UseBubbleTea: true,
	}
	closeKeyInput := func() {}
	if isStdioInputPath(options.InputPath) && deps.openTTY != nil {
		keyInput, err := deps.openTTY()
		if err == nil {
			runOptions.KeyInput = keyInput
			closeKeyInput = func() {
				_ = keyInput.Close()
			}
		}
	}
	defer closeKeyInput()

	if err := deps.runApp(context.Background(), options, stdin, stdout, runOptions); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func isStdioInputPath(path string) bool {
	return path == "" || path == "-"
}
