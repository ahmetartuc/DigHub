package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmetartuc/dighub/v2/internal/config"
	"github.com/ahmetartuc/dighub/v2/internal/scanner"
	"github.com/fatih/color"
)

// Handler manages output generation
type Handler struct {
	config *config.Config
}

// New creates a new output handler
func New(cfg *config.Config) *Handler {
	return &Handler{
		config: cfg,
	}
}

// Write generates output in the specified format
func (h *Handler) Write(results *scanner.ScanResults) error {
	switch h.config.OutputFormat {
	case "terminal":
		return h.writeTerminal(results)
	case "json":
		return h.writeJSON(results)
	case "csv":
		return h.writeCSV(results)
	case "html":
		return h.writeHTML(results)
	default:
		return fmt.Errorf("unsupported output format: %s", h.config.OutputFormat)
	}
}

// writeTerminal displays results in the terminal
func (h *Handler) writeTerminal(results *scanner.ScanResults) error {
	if h.config.Quiet {
		// Only show URLs in quiet mode
		for _, match := range results.Matches {
			fmt.Println(match.URL)
		}
		return nil
	}

	// Group matches by priority
	highMatches := []scanner.Match{}
	mediumMatches := []scanner.Match{}
	lowMatches := []scanner.Match{}

	for _, match := range results.Matches {
		switch match.Dork.Priority {
		case "high":
			highMatches = append(highMatches, match)
		case "medium":
			mediumMatches = append(mediumMatches, match)
		case "low":
			lowMatches = append(lowMatches, match)
		}
	}

	// Display by priority
	if len(highMatches) > 0 {
		fmt.Println()
		fmt.Println(color.RedString("═══ HIGH PRIORITY FINDINGS ═══"))
		h.displayMatches(highMatches)
	}

	if len(mediumMatches) > 0 {
		fmt.Println()
		fmt.Println(color.YellowString("═══ MEDIUM PRIORITY FINDINGS ═══"))
		h.displayMatches(mediumMatches)
	}

	if len(lowMatches) > 0 && h.config.Verbose {
		fmt.Println()
		fmt.Println(color.CyanString("═══ LOW PRIORITY FINDINGS ═══"))
		h.displayMatches(lowMatches)
	}

	return nil
}

// displayMatches shows a list of matches
func (h *Handler) displayMatches(matches []scanner.Match) {
	for _, match := range matches {
		fmt.Printf("\n%s %s\n",
			color.BlueString("Dork:"),
			color.WhiteString(match.Dork.Pattern))
		fmt.Printf("%s %s\n",
			color.BlueString("Category:"),
			color.CyanString(match.Dork.Category))
		fmt.Printf("%s %s\n",
			color.BlueString("Repository:"),
			color.GreenString(match.Repository))
		fmt.Printf("%s %s\n",
			color.BlueString("File:"),
			color.YellowString(match.Path))
		fmt.Printf("%s %s\n",
			color.BlueString("URL:"),
			match.URL)
		for _, line := range snippetLines(match.Fragments) {
			fmt.Printf("    %s %s\n", color.HiBlackString("│"), line)
		}
	}
}

// Snippets are cut down so a minified file can't flood the report
const (
	maxSnippetLines = 3
	maxSnippetWidth = 160
)

// snippetLines returns the first non-empty matched lines, trimmed for display
func snippetLines(fragments []string) []string {
	var lines []string
	for _, f := range fragments {
		for _, line := range strings.Split(f, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if r := []rune(line); len(r) > maxSnippetWidth {
				line = string(r[:maxSnippetWidth]) + "…"
			}
			lines = append(lines, line)
			if len(lines) == maxSnippetLines {
				return lines
			}
		}
	}
	return lines
}

// snippet joins snippetLines for single-field formats
func snippet(fragments []string) string {
	return strings.Join(snippetLines(fragments), "\n")
}

// writeJSON exports results to JSON
func (h *Handler) writeJSON(results *scanner.ScanResults) error {
	file, err := os.Create(h.config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	output := map[string]interface{}{
		"scan_info": map[string]interface{}{
			"platform":    results.Platform,
			"target":      results.Target,
			"scan_date":   results.ScanDate,
			"duration":    results.Duration,
			"total_dorks": results.TotalDorks,
		},
		"summary": map[string]interface{}{
			"total_matches":   results.TotalMatches,
			"unique_files":    results.UniqueFiles,
			"high_priority":   results.HighPriority,
			"medium_priority": results.MediumPriority,
			"low_priority":    results.LowPriority,
			"truncated_dorks": results.Truncated,
			"skipped_dorks":   len(results.Skipped),
		},
		"findings": results.Matches,
		"skipped":  results.Skipped,
	}

	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	results.OutputFile = h.config.OutputFile
	return nil
}

// writeCSV exports results to CSV
func (h *Handler) writeCSV(results *scanner.ScanResults) error {
	file, err := os.Create(h.config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"Priority",
		"Category",
		"Dork Pattern",
		"Description",
		"Repository",
		"File Path",
		"URL",
		"Snippet",
		"Score",
		"Timestamp",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write matches
	for _, match := range results.Matches {
		row := []string{
			match.Dork.Priority,
			match.Dork.Category,
			match.Dork.Pattern,
			match.Dork.Description,
			match.Repository,
			match.Path,
			match.URL,
			snippet(match.Fragments),
			fmt.Sprintf("%.2f", match.Score),
			match.Timestamp.Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	results.OutputFile = h.config.OutputFile
	return nil
}

// writeHTML exports results to HTML
func (h *Handler) writeHTML(results *scanner.ScanResults) error {
	file, err := os.Create(h.config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create HTML file: %w", err)
	}
	defer file.Close()

	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"priorityColor": func(priority string) string {
			switch priority {
			case "high":
				return "#dc3545"
			case "medium":
				return "#ffc107"
			case "low":
				return "#17a2b8"
			default:
				return "#6c757d"
			}
		},
		"priorityBadge": func(priority string) string {
			return strings.ToUpper(priority)
		},
		"snippet": snippet,
	}).Parse(htmlTemplate))

	if err := tmpl.Execute(file, results); err != nil {
		return fmt.Errorf("failed to generate HTML: %w", err)
	}

	absPath, _ := filepath.Abs(h.config.OutputFile)
	results.OutputFile = absPath
	return nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dighub Scan Report - {{.Target}}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .header {
            background: white;
            border-radius: 12px;
            padding: 30px;
            margin-bottom: 20px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
        }
        .header h1 {
            color: #667eea;
            margin-bottom: 10px;
            font-size: 2.5em;
        }
        .header .subtitle {
            color: #6c757d;
            font-size: 1.1em;
        }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .stat-card {
            background: white;
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            text-align: center;
        }
        .stat-number {
            font-size: 2.5em;
            font-weight: bold;
            color: #667eea;
        }
        .stat-label {
            color: #6c757d;
            margin-top: 5px;
            font-size: 0.9em;
        }
        .findings {
            background: white;
            border-radius: 12px;
            padding: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
        }
        .findings h2 {
            color: #333;
            margin-bottom: 20px;
            font-size: 1.8em;
        }
        .finding-item {
            border: 1px solid #e9ecef;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 15px;
            transition: all 0.3s;
        }
        .finding-item:hover {
            box-shadow: 0 5px 15px rgba(0,0,0,0.1);
            transform: translateY(-2px);
        }
        .finding-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
        }
        .finding-dork {
            font-family: 'Courier New', monospace;
            font-weight: bold;
            color: #333;
            font-size: 1.1em;
        }
        .priority-badge {
            padding: 5px 15px;
            border-radius: 20px;
            color: white;
            font-weight: bold;
            font-size: 0.85em;
        }
        .finding-details {
            display: grid;
            grid-template-columns: auto 1fr;
            gap: 10px;
            margin-top: 15px;
        }
        .detail-label {
            font-weight: bold;
            color: #6c757d;
        }
        .detail-value {
            color: #333;
        }
        .detail-value a {
            color: #667eea;
            text-decoration: none;
        }
        .detail-value a:hover {
            text-decoration: underline;
        }
        .category-tag {
            display: inline-block;
            background: #e9ecef;
            padding: 3px 10px;
            border-radius: 4px;
            font-size: 0.85em;
            color: #495057;
        }
        .snippet {
            margin-top: 12px;
            padding: 10px 12px;
            background: #f8f9fa;
            border-radius: 6px;
            font-family: 'Courier New', monospace;
            font-size: 0.85em;
            color: #333;
            white-space: pre-wrap;
            word-break: break-all;
        }
        .skipped {
            background: white;
            border-left: 5px solid #dc3545;
            border-radius: 12px;
            padding: 20px 30px;
            margin-bottom: 20px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
        }
        .skipped h2 {
            color: #dc3545;
            margin-bottom: 10px;
        }
        .skipped li {
            margin: 6px 0 0 20px;
            font-family: 'Courier New', monospace;
            font-size: 0.9em;
        }
        .footer {
            text-align: center;
            color: white;
            margin-top: 30px;
            padding: 20px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔍 Dighub Scan Report</h1>
            <div class="subtitle">Platform: {{.Platform}} • Target: {{.Target}} • Scan Date: {{.ScanDate.Format "2006-01-02 15:04:05"}} • Duration: {{.Duration}}</div>
        </div>

        <div class="stats">
            <div class="stat-card">
                <div class="stat-number">{{.TotalMatches}}</div>
                <div class="stat-label">Total Findings</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">{{.UniqueFiles}}</div>
                <div class="stat-label">Unique Files</div>
            </div>
            <div class="stat-card">
                <div class="stat-number" style="color: #dc3545;">{{.HighPriority}}</div>
                <div class="stat-label">High Priority</div>
            </div>
            <div class="stat-card">
                <div class="stat-number" style="color: #ffc107;">{{.MediumPriority}}</div>
                <div class="stat-label">Medium Priority</div>
            </div>
            <div class="stat-card">
                <div class="stat-number" style="color: #17a2b8;">{{.LowPriority}}</div>
                <div class="stat-label">Low Priority</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">{{.TotalDorks}}</div>
                <div class="stat-label">Dorks Scanned</div>
            </div>
        </div>

        {{if .Skipped}}
        <div class="skipped">
            <h2>{{len .Skipped}} dorks were skipped</h2>
            <p>These searches failed, so their results are unknown. Re-run the scan to cover them.</p>
            <ul>
            {{range .Skipped}}<li>{{.Pattern}} — {{.Reason}}</li>
            {{end}}
            </ul>
        </div>
        {{end}}

        <div class="findings">
            <h2>Detailed Findings</h2>
            {{range .Matches}}
            <div class="finding-item">
                <div class="finding-header">
                    <span class="finding-dork">{{.Dork.Pattern}}</span>
                    <span class="priority-badge" style="background-color: {{priorityColor .Dork.Priority}};">
                        {{priorityBadge .Dork.Priority}}
                    </span>
                </div>
                <div class="finding-details">
                    <span class="detail-label">Category:</span>
                    <span class="detail-value"><span class="category-tag">{{.Dork.Category}}</span></span>
                    
                    <span class="detail-label">Description:</span>
                    <span class="detail-value">{{.Dork.Description}}</span>
                    
                    <span class="detail-label">Repository:</span>
                    <span class="detail-value">{{.Repository}}</span>
                    
                    <span class="detail-label">File Path:</span>
                    <span class="detail-value">{{.Path}}</span>
                    
                    <span class="detail-label">URL:</span>
                    <span class="detail-value"><a href="{{.URL}}" target="_blank">{{.URL}}</a></span>
                </div>
                {{with snippet .Fragments}}<div class="snippet">{{.}}</div>{{end}}
            </div>
            {{end}}
        </div>

        <div class="footer">
            <p>Generated by Dighub - Advanced GitHub & GitLab Dorking & Secret Hunting Tool</p>
            <p>⚠️ This tool is for authorized security research only</p>
        </div>
    </div>
</body>
</html>
`
