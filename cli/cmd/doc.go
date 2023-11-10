package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

const documentationBasepath = "./docs"

var DocCmd = &cobra.Command{
	Use:   "doc",
	Short: fmt.Sprintf("Build documentation under %s", documentationBasepath),
	Long:  fmt.Sprintf(`Generate documentation for command line usage under /tmp/fishler%s`, documentationBasepath),
	Run: func(cmd *cobra.Command, args []string) {
		err := os.MkdirAll(documentationBasepath, os.ModePerm)
		if err != nil {
			logrus.Fatal(err)
		}

		err = doc.GenMarkdownTree(cmd.Root(), documentationBasepath)
		if err != nil {
			log.Fatal(err)
		}
	},
}
