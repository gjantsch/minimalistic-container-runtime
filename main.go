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
	fmt.Printf("[runtime] running: %v\n", os.Args)
	arg, args := pop(os.Args[1:])
	switch arg {
	case "child":
		container.RunChild(args)
	case "run":
		if len(args) == 0 {
			fmt.Printf("[runtime] no command provided\n")
			os.Exit(1)
		}
		fmt.Printf("[runtime] command to run: %s\n", args[0])
		container.RunParent(args)

	default:
		fmt.Printf("usage: container run <cmd>\n")
		os.Exit(1)
	}

}
