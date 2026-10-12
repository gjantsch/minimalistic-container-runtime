package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gjantsch/container/cmd/container"
)

const Version = "V 0.1.0"

func pop(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}

// parseMem converts strings like "64m", "128k", "1g" to bytes.
func parseMem(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "0" {
		return 0, nil
	}
	multipliers := map[byte]int64{'k': 1024, 'm': 1024 * 1024, 'g': 1024 * 1024 * 1024}
	last := s[len(s)-1]
	if mul, ok := multipliers[last]; ok {
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid mem value %q: %w", s, err)
		}
		return n * mul, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

func main() {
	arg, args := pop(os.Args[1:])
	switch arg {
	case "child":
		container.RunChild(args)
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		memFlag := fs.String("mem", "0", "memory limit e.g. 64m")
		cpuFlag := fs.Float64("cpu", 0, "cpu limit e.g. 0.5")
		fs.Parse(args)

		cmd := fs.Args()
		if len(cmd) == 0 {
			fmt.Printf("usage: container run [--mem=64m] [--cpu=0.5] <cmd>\n")
			os.Exit(1)
		}

		memBytes, err := parseMem(*memFlag)
		if err != nil {
			fmt.Printf("[runtime] invalid --mem: %v\n", err)
			os.Exit(1)
		}

		const cpuPeriod = 100000
		cpuQuota := int64(*cpuFlag * cpuPeriod)

		cfg := container.Config{
			MemBytes:  memBytes,
			CPUQuota:  cpuQuota,
			CPUPeriod: cpuPeriod,
			Cmd:       cmd,
		}

		fmt.Printf("[runtime] launching: %s\n", cfg.Cmd[0])
		container.RunParent(cfg)

	default:
		fmt.Printf("usage: container run <cmd>\n")
		os.Exit(1)
	}
}
