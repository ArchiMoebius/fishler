package cli

// Execute is the entry point for the cli
// called from main
func Execute() error {
	return RootCmd.Execute()
}
