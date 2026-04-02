package main

import (
	"os"

	"github.com/OverseedAI/overcf/internal/cli"
)

func main() {
	code := cli.Execute()
	os.Exit(code)
}
