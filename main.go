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
	fmt.Printf("[runtime] starting with args: %v\n", os.Args)
	arg, args := pop(os.Args[1:])
	fmt.Printf("[runtime] remaining arg, args: %s, %v\n", arg, args)
	switch arg {
	case "child":
		fmt.Printf("[runtime] running as child\n")
		container.RunChild(args)
	case "run":
		fmt.Printf("[runtime] run mode\n")
		if arg == "" {
			fmt.Printf("[runtime] no command provided\n")
			os.Exit(1)
		}
		fmt.Printf("[runtime] command to run: %s\n", arg)

	default:
		fmt.Printf("[runtime] %s - ready\n", Version)
		container.RunParent(os.Args[1:])
	}

}
