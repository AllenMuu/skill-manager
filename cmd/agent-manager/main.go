// agent-manager discovers and manages local agent resources.
package main

import (
	"fmt"
	"os"

	"github.com/AllenMuu/skill-manager/internal/cli"
)

func main() {
	if err := cli.NewAgentManagerCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
