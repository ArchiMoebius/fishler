package main

import (
	"os"

	"github.com/archimoebius/fishler/cli"
	"github.com/archimoebius/fishler/util"
)

func main() {
	if err := cli.Execute(); err != nil {
		util.Logger.Error(err)
		os.Exit(1)
	}
}
