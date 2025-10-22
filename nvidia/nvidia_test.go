package main

import (
	"os"
	"testing"
)

func TestNvidiaExecution(t *testing.T) {
	// Set up temporary flags for testing
	os.Args = []string{"nvidia", "--trace-dir=data/simple-trace-example", "--device=H100"}

	// Run the main function
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Execution failed with panic: %v", r)
		}
	}()

	main() // Execute the main function

	// If no panic or error occurs, the test passes
	t.Log("nvidia executed successfully")
}
