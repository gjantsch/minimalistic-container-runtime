package main

import (
	"fmt"
	"os"

	"github.com/gjantsch/container/cmd/container"
)

const Version = "V 0.1.0"

func main() {
	arg := ""
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}
	if arg == "child" {
		fmt.Printf("[runtime] running as child\n")
		container.RunChild()
	} else {
		fmt.Printf("[runtime] %s - ready\n", Version)
		container.RunParent()
	}

}
