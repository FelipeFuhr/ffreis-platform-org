package main

import (
	"fmt"
	"os"

	"github.com/ffreis/platform-org/cmd"
)

var execute = cmd.Execute
var exitFunc = os.Exit

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitFunc(1)
	}
}
