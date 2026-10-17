// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	memoclient "github.com/aj-nt/vassago-sdk/client"
)

// cmdMemoryExport handles "vassago export" — exports all memories to JSON.
func cmdMemoryExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	addr := fs.String("addr", "localhost:50051", "Daemon address (host:port)")
	output := fs.String("output", "", "Output file path (default: vassago-export-<timestamp>.json)")
	target := fs.String("target", "", "Target filter: memory, user, or empty for both")
	fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: *addr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer c.Close()

	// List all memories (both targets if no filter)
	entries, err := c.ListMemories(ctx, *target, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing memories: %v\n", err)
		os.Exit(1)
	}

	// Convert to export format
	exports := make([]memoryExport, len(entries))
	for i, e := range entries {
		exports[i] = memoryExport{
			ID:          e.Id,
			Target:      e.Target,
			Category:    e.Category,
			Key:         e.Key,
			Content:     e.Content,
			Priority:    e.Priority,
			SourceAgent: e.SourceAgent,
			CreatedAt:   e.CreatedAt,
			UpdatedAt:   e.UpdatedAt,
			AccessedAt:  e.AccessedAt,
			ExpiresAt:   e.ExpiresAt,
		}
	}

	// Default output path
	if *output == "" {
		*output = fmt.Sprintf("vassago-export-%s.json", time.Now().Format("20060102-150405"))
	}

	data, err := json.MarshalIndent(exports, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling export: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*output, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Exported %d memories to %s\n", len(exports), *output)
}

// cmdMemoryImport handles "vassago import" — imports memories from JSON.
func cmdMemoryImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	addr := fs.String("addr", "localhost:50051", "Daemon address (host:port)")
	skipExisting := fs.Bool("skip-existing", true, "Skip entries with existing target+key (true by default for safety)")
	fs.Parse(args)

	inputPath := fs.Arg(0)
	if inputPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: vassago import <file.json> [--skip-existing=true/false]")
		os.Exit(1)
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var exports []memoryExport
	if err := json.Unmarshal(data, &exports); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	c, err := memoclient.Connect(ctx, memoclient.Config{Address: *addr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer c.Close()

	imported := 0
	skipped := 0
	var errors []string

	for _, e := range exports {
		// Check if entry exists (when skipExisting)
		if *skipExisting && e.Key != "" {
			existing, err := c.SearchMemories(ctx, e.Key, e.Target, 1)
			if err == nil && len(existing) > 0 {
				// Check if exact key match
				for _, ex := range existing {
					if ex.Key == e.Key && ex.Target == e.Target {
						skipped++
						continue
					}
				}
			}
		}

		_, err := c.AddMemory(ctx, e.Target, e.Category, e.Key, e.Content, e.Priority, e.SourceAgent)
		if err != nil {
			errors = append(errors, fmt.Sprintf("key=%s: %v", e.Key, err))
			skipped++
			continue
		}
		imported++
	}

	fmt.Printf("Import complete:\n")
	fmt.Printf("  Imported: %d\n", imported)
	fmt.Printf("  Skipped: %d\n", skipped)
	if len(errors) > 0 {
		fmt.Printf("  Errors (%d):\n", len(errors))
		for _, e := range errors {
			fmt.Printf("    %s\n", e)
		}
	}
}

// memoryExport is the JSON-serializable format for memory export.
type memoryExport struct {
	ID          string `json:"id"`
	Target      string `json:"target"`
	Category    string `json:"category"`
	Key         string `json:"key"`
	Content     string `json:"content"`
	Priority    int32  `json:"priority"`
	SourceAgent string `json:"source_agent"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	AccessedAt  int64  `json:"accessed_at"`
	ExpiresAt   int64  `json:"expires_at"`
}
