// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/aj-nt/mnemosyne/daemon/internal/migration"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
)

func main() {
	source := flag.String("source", "", "Path to Hermes state.db to import")
	target := flag.String("target", "", "Path to Mnemosyne database (default: ~/.mnemosyne/mnemosyne.db)")
	agent := flag.String("agent", "hermes-import", "Agent ID for imported sessions and memories (default: hermes-import)")
	dryRun := flag.Bool("dry-run", false, "Show what would be imported without actually importing")
	flag.Parse()

	if *source == "" {
		fmt.Fprintln(os.Stderr, "Error: --source is required")
		flag.Usage()
		os.Exit(1)
	}

	// Default target path
	if *target == "" {
		home, _ := os.UserHomeDir()
		*target = home + "/.mnemosyne/mnemosyne.db"
	}

	log.Printf("Importing from Hermes: %s", *source)
	log.Printf("Target Mnemosyne: %s", *target)
	log.Printf("Agent ID: %s", *agent)

	if *dryRun {
		log.Println("DRY RUN — no changes will be made")
		// TODO: implement dry-run mode
		os.Exit(0)
	}

	// Open Mnemosyne store
	mnemoStore, err := store.New(*target)
	if err != nil {
		log.Fatalf("Failed to open Mnemosyne database: %v", err)
	}
	defer mnemoStore.Close()

	result, err := migration.ImportHermesDB(mnemoStore, *source, *agent)
	if err != nil {
		log.Fatalf("Import failed: %v", err)
	}

	log.Printf("Import complete:")
	log.Printf("  Memories: %d read, %d imported, %d skipped", result.MemoriesRead, result.MemoriesImported, result.MemoriesSkipped)
	log.Printf("  Sessions: %d read, %d imported, %d skipped", result.SessionsRead, result.SessionsImported, result.SessionsSkipped)
	log.Printf("  Messages: %d read, %d imported", result.MessagesRead, result.MessagesImported)
	log.Printf("  Agents created: %d", result.AgentsCreated)

	if len(result.Errors) > 0 {
		log.Printf("  Errors (%d):", len(result.Errors))
		for _, e := range result.Errors {
			log.Printf("    %s", e)
		}
	}

	// Run expiry sweep to clean up any expired memories from the import
	removed, err := mnemoStore.ExpireStaleMemories()
	if err != nil {
		log.Printf("Warning: expiry sweep failed: %v", err)
	} else if removed > 0 {
		log.Printf("  Expired memories removed: %d", removed)
	}
}