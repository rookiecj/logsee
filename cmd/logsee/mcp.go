package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"git.inpt.fr/42dottools/log/internal/view/mcp"
)

// runMCP serves the Model Context Protocol over stdio on one connection.
// Returns when stdin closes or ctx is cancelled.
func runMCP() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := mcp.NewServer()
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		return fmt.Errorf("mcp serve: %w", err)
	}
	return nil
}
