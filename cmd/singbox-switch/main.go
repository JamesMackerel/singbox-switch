package main

import (
	"fmt"
	"os"

	"singbox-switch/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "singbox-switch:", err)
		os.Exit(1)
	}
}
