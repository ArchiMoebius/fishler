package main

import (
	"os"

	"github.com/ArchiMoebius/fishler/cli"
	"github.com/ArchiMoebius/fishler/util"
)

func main() {
	if err := cli.Execute(); err != nil {
		util.Logger.Error(err)
		os.Exit(1)
	}
}
