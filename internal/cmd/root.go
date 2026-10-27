package cmd

import (
	"fmt"
	"strings"

	"github.com/ahmetartuc/dighub/v2/internal/config"
	"github.com/ahmetartuc/dighub/v2/internal/logger"
	"github.com/ahmetartuc/dighub/v2/internal/output"
	"github.com/ahmetartuc/dighub/v2/internal/scanner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	cfg     *config.Config
	version string
)

var rootCmd = &cobra.Command{
	Use:   "dighub",
	Short: "Advanced GitHub & GitLab Dorking & Secret Hunting Tool",
	Long: `Dighub is a powerful CLI tool that performs advanced code search dorking
on GitHub and GitLab (including self-hosted instances) to detect exposed
secrets, credentials, webhooks and sensitive files inside repositories.`,
	Version: version,
	RunE:    run,
}

func init() {
	cfg = &config.Config{}

	rootCmd.Flags().StringVarP(&cfg.Platform, "platform", "P", "github", "Platform to search: github, gitlab")
	rootCmd.Flags().StringVar(&cfg.BaseURL, "url", "", "Instance URL for self-hosted GitLab or GitHub Enterprise")
	rootCmd.Flags().StringVarP(&cfg.Organization, "org", "o", "", "Organization (GitHub) or group (GitLab) to scan")
	rootCmd.Flags().StringVarP(&cfg.Token, "token", "t", "", "Personal access token for the platform (required)")
	rootCmd.Flags().StringVarP(&cfg.User, "user", "u", "", "User to scan (alternative to org)")

	rootCmd.Flags().StringVarP(&cfg.OutputFormat, "output", "f", "terminal", "Output format: terminal, json, csv, html")
	rootCmd.Flags().StringVarP(&cfg.OutputFile, "out-file", "w", "", "Output file path (default: ./dighub-results.[format])")

	rootCmd.Flags().StringSliceVarP(&cfg.IncludeDorks, "include", "i", []string{}, "Include specific dorks (comma-separated)")
	rootCmd.Flags().StringSliceVarP(&cfg.ExcludeDorks, "exclude", "e", []string{}, "Exclude specific dorks (comma-separated)")
	rootCmd.Flags().StringVarP(&cfg.Priority, "priority", "p", "all", "Priority level: all, high, medium, low")

	rootCmd.Flags().IntVarP(&cfg.Workers, "workers", "W", 5, "Number of concurrent workers")
	rootCmd.Flags().IntVarP(&cfg.RateLimit, "rate-limit", "r", 0, "Requests per minute (0: platform default, GitHub 10, GitLab 30)")
	rootCmd.Flags().IntVarP(&cfg.Delay, "delay", "d", 0, "Extra delay per worker after each dork (seconds)")
	rootCmd.Flags().IntVarP(&cfg.MaxPages, "max-pages", "m", 1, "Result pages to fetch per dork, 100 results each (1-10)")

	rootCmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Verbose output")
	rootCmd.Flags().BoolVarP(&cfg.Quiet, "quiet", "q", false, "Quiet mode (only show matches)")
	rootCmd.Flags().BoolVarP(&cfg.NoColor, "no-color", "n", false, "Disable colored output")
	rootCmd.Flags().BoolVarP(&cfg.SaveProgress, "save-progress", "s", false, "Save progress to resume later")
	rootCmd.Flags().StringVar(&cfg.ProgressFile, "progress-file", ".dighub-progress.json", "Progress file path")

	rootCmd.MarkFlagRequired("token")
}

func Execute(ver string) error {
	version = ver
	rootCmd.Version = ver
	return rootCmd.Execute()
}

func run(cmd *cobra.Command, args []string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if cfg.NoColor {
		color.NoColor = true
	}

	log := logger.New(cfg.Verbose, cfg.Quiet)

	if !cfg.Quiet {
		printBanner(version)
	}

	log.Info("Initializing scanner...")
	scn, err := scanner.New(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to initialize scanner: %w", err)
	}

	log.Info("Starting scan...")
	target := cfg.Organization
	if target == "" {
		target = cfg.User
	}
	log.Info("Platform: %s", cfg.Platform)
	log.Info("Target: %s", target)
	log.Info("Workers: %d", cfg.Workers)
	log.Info("Output format: %s", cfg.OutputFormat)

	results, err := scn.Scan()
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	log.Info("Generating output...")
	outputHandler := output.New(cfg)
	if err := outputHandler.Write(results); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	if !cfg.Quiet {
		printSummary(results, log)
	}

	return nil
}

func printBanner(ver string) {
	banner := `
    ____  _       __  __      __  
   / __ \(_)___ _/ / / /_  __/ /_ 
  / / / / / __ '/ /_/ / / / / __ \
 / /_/ / / /_/ / __  / /_/ / /_/ /
/_____/_/\__, /_/ /_/\__,_/_.___/ 
        /____/                     
`
	fmt.Println(color.CyanString(banner))
	fmt.Printf("%s %s\n", color.YellowString("Version:"), ver)
	fmt.Printf("%s Advanced GitHub & GitLab Dorking & Secret Hunting Tool\n", color.GreenString("=>"))
	fmt.Println(strings.Repeat("─", 50))
}

func printSummary(results *scanner.ScanResults, log *logger.Logger) {
	fmt.Println()
	fmt.Println(strings.Repeat("═", 50))
	fmt.Println(color.CyanString("SCAN SUMMARY"))
	fmt.Println(strings.Repeat("═", 50))

	fmt.Printf("%s %d\n", color.BlueString("Total Dorks Scanned:"), results.TotalDorks)
	fmt.Printf("%s %d\n", color.GreenString("Matches Found:"), results.TotalMatches)
	fmt.Printf("%s %d\n", color.YellowString("Unique Files:"), results.UniqueFiles)
	fmt.Printf("%s %d\n", color.RedString("High Priority:"), results.HighPriority)
	fmt.Printf("%s %d\n", color.MagentaString("Medium Priority:"), results.MediumPriority)
	fmt.Printf("%s %d\n", color.CyanString("Low Priority:"), results.LowPriority)
	fmt.Printf("%s %s\n", color.BlueString("Duration:"), results.Duration)
	if results.Truncated > 0 {
		fmt.Printf("%s %d (raise --max-pages)\n", color.YellowString("Truncated Dorks:"), results.Truncated)
	}
	if len(results.Skipped) > 0 {
		fmt.Printf("%s %d\n", color.RedString("Skipped Dorks:"), len(results.Skipped))
	}

	fmt.Println(strings.Repeat("═", 50))

	if len(results.Skipped) > 0 {
		log.Warning("%d dorks could not be searched, so their results are unknown:", len(results.Skipped))
		for _, sk := range results.Skipped {
			log.Warning("  %s: %s", sk.Pattern, sk.Reason)
		}
	}

	if results.TotalMatches > 0 {
		log.Success("✓ Scan completed successfully!")
		if results.OutputFile != "" {
			log.Success("Results saved to: %s", results.OutputFile)
		}
	} else if len(results.Skipped) > 0 {
		log.Warning("No matches in the dorks that ran, but the skipped ones were not checked.")
	} else {
		log.Info("No sensitive data found in the target.")
	}
}
