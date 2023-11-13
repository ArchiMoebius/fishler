package main

import (
	"log"

	"github.com/archimoebius/fishler/cli"
)

func main() {

	if err := cli.Execute(); err != nil {
		log.Fatal(err)
	}
}
