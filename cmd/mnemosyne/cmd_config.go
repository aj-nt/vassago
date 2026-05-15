// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// cmdConfig handles "mnemosyne config" — configuration management.
func cmdConfig(args []string) {
	if len(args) < 1 {
		fmt.Println(`Usage: mnemosyne config <subcommand>

Subcommands:
  validate    Validate daemon and agent configs
  show        Display current configurations
  edit        Open configs in $EDITOR
  dirs        Show data directory paths

Use "mnemosyne config <subcommand> --help" for more information.`)
		return
	}

	switch args[0] {
	case "validate":
		cmdConfigValidateDeep(args[1:])
	case "show":
		cmdConfigShow(args[1:])
	case "edit":
		cmdConfigEdit(args[1:])
	case "dirs":
		cmdConfigDirs(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand: %s\n", args[0])
	}
}

func cmdConfigValidate(args []string) {
	home, _ := userHomeDir()
	daemonCfg := home + "/.mnemosyne/config.yaml"
	agentCfg := home + "/.mnemosyne/agent.yaml"

	fmt.Println("Validating configurations...")
	fmt.Println()

	// Validate daemon config
	fmt.Printf("Daemon config: %s\n", daemonCfg)
	if err := validateDaemonConfig(daemonCfg); err != nil {
		fmt.Printf("  ✗ %v\n", err)
	} else {
		fmt.Println("  ✓ Valid")
	}

	// Validate agent config
	fmt.Printf("Agent config:  %s\n", agentCfg)
	if err := validateAgentConfig(agentCfg); err != nil {
		fmt.Printf("  ✗ %v\n", err)
	} else {
		fmt.Println("  ✓ Valid")
	}
}

func cmdConfigShow(args []string) {
	home, _ := userHomeDir()
	fmt.Println("Data directory:", home+"/.mnemosyne")
	fmt.Println()

	// Show daemon config
	daemonCfg := home + "/.mnemosyne/config.yaml"
	fmt.Printf("=== Daemon Config (%s) ===\n", daemonCfg)
	if data, err := readFile(daemonCfg); err == nil {
		fmt.Println(string(data))
	} else {
		fmt.Printf("  (not found: %v)\n", err)
	}

	// Show agent config
	agentCfg := home + "/.mnemosyne/agent.yaml"
	fmt.Printf("=== Agent Config (%s) ===\n", agentCfg)
	if data, err := readFile(agentCfg); err == nil {
		fmt.Println(string(data))
	} else {
		fmt.Printf("  (not found: %v)\n", err)
	}
}

func cmdConfigEdit(args []string) {
	editor := "vim"
	if e := getEnv("EDITOR"); e != "" {
		editor = e
	}
	if e := getEnv("VISUAL"); e != "" {
		editor = e
	}

	home, _ := userHomeDir()
	files := []string{
		home + "/.mnemosyne/config.yaml",
		home + "/.mnemosyne/agent.yaml",
	}

	cmd := exec.Command(editor, files...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening editor: %v\n", err)
	}
}

func cmdConfigDirs(args []string) {
	home, _ := userHomeDir()
	base := home + "/.mnemosyne"
	dirs := []string{
		base,
		base + "/screenshots",
		base + "/agents",
		base + "/pastes",
		base + "/personalities",
		base + "/generated_images",
	}
	fmt.Println("Mnemosyne directories:")
	for _, d := range dirs {
		exists := "✓"
		if !fileExists(d) {
			exists = "✗ (not created)"
		}
		fmt.Printf("  %s  %s\n", d, exists)
	}
	fmt.Println()
	fmt.Println("Config files:")
	for _, f := range []string{base + "/config.yaml", base + "/agent.yaml"} {
		exists := "✓"
		if !fileExists(f) {
			exists = "✗ (missing — run 'mnemosyne init')"
		}
		fmt.Printf("  %s  %s\n", f, exists)
	}
	fmt.Println()
	fmt.Println("System:")
	fmt.Printf("  OS:      %s\n", runtime.GOOS)
	fmt.Printf("  Arch:    %s\n", runtime.GOARCH)
	fmt.Printf("  Home:    %s\n", home)
}
