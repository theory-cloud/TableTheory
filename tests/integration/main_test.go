package integration

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain ensures integration tests never run during -short test passes.
func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		fmt.Println("Skipping integration tests in -short mode")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
