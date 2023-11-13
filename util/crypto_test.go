package util

import (
	"os"
	"testing"

	config "github.com/archimoebius/fishler/cli/config/serve"
	"github.com/spf13/cobra"
)

var ServeCmd = &cobra.Command{}

func init() {
	os.Setenv("FISHLER_CRYPTO_BASEPATH", "/tmp/")
	config.CommandInit(ServeCmd)
	config.Load()
}

func TestGetFishlerPrivateKey(t *testing.T) {

	path, err := GetFishlerPrivateKey()

	if path == "" {
		t.Fatalf("Failed to get private key from /tmp/ or generate it in /tmp...")
	}

	if err != nil {
		t.Fatal(err)
	}

	if GetFishlerPrivateKeyPath() == "" {
		t.Fatalf("Failed to get private key from /tmp/ or generate it in /tmp...")
	}

	_, err = GetKeySigner()

	if err != nil {
		t.Fatal(err)
	}
}
