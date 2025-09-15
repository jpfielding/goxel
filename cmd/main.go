package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jpfielding/goxel/pkg/logging"
)

var (
	GitSHA string = "NA"
)

func main() {
	// register sigterm for graceful shutdown
	ctx, cnc := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cnc()
	go func() {
		defer cnc() // this cnc is from notify and removes the signal so subsequent ctrl-c will restore kill functions
		<-ctx.Done()
	}()
	slog.SetDefault(logging.Logger(os.Stdout, false, slog.LevelInfo))
	ctx = logging.AppendCtx(ctx,
		slog.Group("goxel",
			slog.String("git", GitSHA),
		))
	NewRoot(ctx, GitSHA).Execute()
}

func NewRoot(ctx context.Context, gitsha string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prosightctl",
		Short: "a CLI to manage clearscan configuration/validation",
		Long:  "the long story",
		Run: func(cmd *cobra.Command, args []string) {
			printCommandTree(cmd, 0)
		},
	}
	cmd.AddCommand(
		NewVersionCmd(ctx, gitsha),
	)
	return cmd
}

func printCommandTree(cmd *cobra.Command, indent int) {
	fmt.Println(strings.Repeat("\t", indent), cmd.Use+":", cmd.Short)
	for _, subCmd := range cmd.Commands() {
		printCommandTree(subCmd, indent+1)
	}
}

func NewVersionCmd(ctx context.Context, gitsha string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "git sha for this build",
		Long:  "git sha for this build",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(gitsha)
		},
	}
	return cmd
}
