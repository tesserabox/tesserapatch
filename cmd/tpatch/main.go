package main

import (
	"os"

	"github.com/tesserabox/tesserapatch/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
