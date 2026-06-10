package main

import (
	"fmt"
	"log"
)

func main() {
	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("irollhub starting on %s\n", cfg.Listen)
}
