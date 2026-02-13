// Command gendocs generates documentation for the alca CLI.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/bolasblack/alcatraz/internal/cli"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gendocs <markdown|man|completions>")
		os.Exit(1)
	}

	cmd := cli.GetRootCmd()

	switch os.Args[1] {
	case "markdown":
		generateMarkdown(cmd)
	case "man":
		generateMan(cmd)
	case "completions":
		generateCompletions(cmd)
	default:
		fmt.Printf("Unknown format: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func generateMarkdown(cmd *cobra.Command) {
	dir := "docs/commands"
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}

	// Add front matter for static site generators
	filePrepender := func(filename string) string {
		name := filepath.Base(filename)
		base := strings.TrimSuffix(name, filepath.Ext(name))
		title := strings.ReplaceAll(base, "_", " ")
		now := time.Now().Format("2006-01-02")
		return fmt.Sprintf(`---
title: "%s"
date: %s
---

`, title, now)
	}

	// Customize links for web usage
	linkHandler := func(name string) string {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		return "./" + base + ".md"
	}

	if err := doc.GenMarkdownTreeCustom(cmd, dir, filePrepender, linkHandler); err != nil {
		log.Fatalf("Failed to generate markdown: %v", err)
	}

	fmt.Printf("Generated markdown documentation in %s/\n", dir)
}

func generateCompletions(cmd *cobra.Command) {
	dir := "out/completions"
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}

	writeCompletion(filepath.Join(dir, "alca.bash"), func(f *os.File) error {
		return cmd.GenBashCompletionV2(f, true)
	})
	writeCompletion(filepath.Join(dir, "alca.zsh"), func(f *os.File) error {
		return cmd.GenZshCompletion(f)
	})
	writeCompletion(filepath.Join(dir, "alca.fish"), func(f *os.File) error {
		return cmd.GenFishCompletion(f, true)
	})

	fmt.Printf("Generated shell completions in %s/\n", dir)
}

func writeCompletion(path string, gen func(*os.File) error) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to create %s: %v", path, err)
	}
	if err := gen(f); err != nil {
		_ = f.Close()
		log.Fatalf("Failed to generate %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("Failed to close %s: %v", path, err)
	}
}

func generateMan(cmd *cobra.Command) {
	dir := "out/man"
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}

	header := &doc.GenManHeader{
		Title:   "ALCA",
		Section: "1",
		Source:  "Alcatraz",
		Manual:  "Alcatraz Manual",
	}

	if err := doc.GenManTree(cmd, header, dir); err != nil {
		log.Fatalf("Failed to generate man pages: %v", err)
	}

	fmt.Printf("Generated man pages in %s/\n", dir)
}
