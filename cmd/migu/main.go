package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/saurlax/migu-aigpu-cli/internal/cli"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := cli.New(version).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
