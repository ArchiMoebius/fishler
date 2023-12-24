package cli

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

const documentationBasepath = "docs/"

var DocCmd = &cobra.Command{
	Use:   "doc",
	Short: fmt.Sprintf("Build documentation under %s", documentationBasepath),
	Long:  fmt.Sprintf(`Generate documentation for command line usage under %s`, documentationBasepath),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		CallPersistentPreRun(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		err := os.MkdirAll(documentationBasepath, 0750)
		if err != nil {
			log.Fatal(err)
		}

		err = doc.GenMarkdownTree(cmd.Root(), documentationBasepath)
		if err != nil {
			log.Fatal(err)
		}
	},
}
