package main

import (
	"fmt"
	"os"

	"github.com/gjantsch/container/cmd/container"
)

const Version = "V 0.1.0"

func pop(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}

func main() {
	arg, args := pop(os.Args[1:])
	switch arg {
	case "child":
		fmt.Printf("[runtime] running as child\n")
		container.RunChild()
	case "run":
		fmt.Printf("[runtime] run mode\n")
		arg, args = pop(args)
		if arg == "" {
			fmt.Printf("[runtime] no command provided\n")
			os.Exit(1)
		}
		fmt.Printf("[runtime] command to run: %s\n", arg)

	default:
		fmt.Printf("[runtime] %s - ready\n", Version)
		container.RunParent()
	}

}
