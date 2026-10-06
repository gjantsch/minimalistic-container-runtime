package main

import (
	"fmt"
	"os"

	"github.com/gjantsch/container/cmd/container"
)

const Version = "V 0.1.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "child" {
		container.RunChild()
	} else {
		fmt.Printf("[container-runtime] %s - ready\n", Version)
		container.RunParent()
	}

}
