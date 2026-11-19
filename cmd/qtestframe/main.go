// Command qtestframe is the requirements and test management server.
package main

import (
	"os"

	"github.com/rom/qtestframe/internal/app"
)

var version = "dev"

func main() {
	app.Version = version
	os.Exit(app.Run(os.Args[1:], app.Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}))
}
