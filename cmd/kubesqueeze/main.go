package main

import (
	"errors"
	"os"

	"github.com/elft/KubeSqueeze/internal/app"
	"github.com/elft/KubeSqueeze/internal/cli"
	"github.com/elft/KubeSqueeze/internal/output"
)

var version = "dev"

func main() {
	command := cli.Command{Out: os.Stdout, ErrOut: os.Stderr, Handler: app.Run, Version: version}.New()
	if err := command.Execute(); err != nil {
		code := "operation_failed"
		if errors.Is(err, cli.ErrNoHandler) {
			code = "not_configured"
		}
		result := output.Error{Code: code, Message: err.Error()}
		var clusterErr *app.ClusterError
		if errors.As(err, &clusterErr) {
			result.Cluster = &clusterErr.Cluster
		}
		_ = output.Write(os.Stderr, result)
		os.Exit(1)
	}
}
