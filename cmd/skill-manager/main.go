// skill-manager discovers and manages local directory skills.
package main

import (
	"flag"
	"fmt"
)

func main() {
	help := flag.Bool("help", false, "show usage")
	flag.Parse()
	if *help {
		fmt.Println("Usage: skill-manager [command]")
	}
}
