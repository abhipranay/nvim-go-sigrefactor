package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/hellofresh/nvim-go-sigrefactor/internal/analyzer"
	"github.com/hellofresh/nvim-go-sigrefactor/internal/refactor"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gosigrefactor",
	Short: "Go signature refactoring tool",
	Long:  `A semantic Go signature refactoring tool for Neovim integration.`,
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze --file=<file> --offset=<offset>",
	Short: "Analyze signature at cursor position",
	RunE:  runAnalyze,
}

var usagesCmd = &cobra.Command{
	Use:   "usages --file=<file> --offset=<offset>",
	Short: "Find all usages and implementations",
	RunE:  runUsages,
}

var refactorCmd = &cobra.Command{
	Use:   "refactor --file=<file> --offset=<offset> --spec=<json>",
	Short: "Apply refactoring",
	RunE:  runRefactor,
}

var (
	flagFile   string
	flagOffset string
	flagSpec   string
)

func init() {
	// Analyze command
	analyzeCmd.Flags().StringVar(&flagFile, "file", "", "Source file path")
	analyzeCmd.Flags().StringVar(&flagOffset, "offset", "", "Byte offset in file")
	_ = analyzeCmd.MarkFlagRequired("file")
	_ = analyzeCmd.MarkFlagRequired("offset")

	// Usages command
	usagesCmd.Flags().StringVar(&flagFile, "file", "", "Source file path")
	usagesCmd.Flags().StringVar(&flagOffset, "offset", "", "Byte offset in file")
	_ = usagesCmd.MarkFlagRequired("file")
	_ = usagesCmd.MarkFlagRequired("offset")

	// Refactor command
	refactorCmd.Flags().StringVar(&flagFile, "file", "", "Source file path")
	refactorCmd.Flags().StringVar(&flagOffset, "offset", "", "Byte offset in file")
	refactorCmd.Flags().StringVar(&flagSpec, "spec", "", "Refactoring spec as JSON")
	_ = refactorCmd.MarkFlagRequired("file")
	_ = refactorCmd.MarkFlagRequired("offset")
	_ = refactorCmd.MarkFlagRequired("spec")

	rootCmd.AddCommand(analyzeCmd, usagesCmd, refactorCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	offset, err := strconv.Atoi(flagOffset)
	if err != nil {
		return fmt.Errorf("invalid offset: %w", err)
	}

	a := analyzer.New()
	result, err := a.Analyze(flagFile, offset)
	if err != nil {
		return err
	}

	return outputJSON(result)
}

func runUsages(cmd *cobra.Command, args []string) error {
	offset, err := strconv.Atoi(flagOffset)
	if err != nil {
		return fmt.Errorf("invalid offset: %w", err)
	}

	a := analyzer.New()
	result, err := a.Analyze(flagFile, offset)
	if err != nil {
		return err
	}

	// Output just usages
	type usagesResult struct {
		Usages          []analyzer.Usage          `json:"usages"`
		Implementations []analyzer.Implementation `json:"implementations,omitempty"`
	}

	return outputJSON(usagesResult{
		Usages:          result.Usages,
		Implementations: result.Implementations,
	})
}

func runRefactor(cmd *cobra.Command, args []string) error {
	offset, err := strconv.Atoi(flagOffset)
	if err != nil {
		return fmt.Errorf("invalid offset: %w", err)
	}

	var spec analyzer.RefactorSpec
	if err := json.Unmarshal([]byte(flagSpec), &spec); err != nil {
		return fmt.Errorf("invalid spec JSON: %w", err)
	}

	r := refactor.New()
	edits, err := r.Refactor(flagFile, offset, spec)
	if err != nil {
		return err
	}

	return outputJSON(edits)
}

func outputJSON(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
