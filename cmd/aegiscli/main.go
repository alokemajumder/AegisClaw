package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const version = "AegisClaw CLI v0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version)
	case "health":
		checkHealth()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: aegiscli <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  version    Print the CLI version")
	fmt.Println("  health     Check API gateway health (GET /healthz)")
}

func checkHealth() {
	baseURL := os.Getenv("AEGISCLAW_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(baseURL + "/healthz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("API Gateway: %s (status %d)\n", result["status"], resp.StatusCode)
}
