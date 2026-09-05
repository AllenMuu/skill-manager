// skill-manager discovers and manages local directory skills.
package main

import (
	"fmt"
	"os"

	"github.com/AllenMuu/skill-manager/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
