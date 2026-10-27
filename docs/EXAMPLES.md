# Dighub Usage Examples

This document provides practical examples of using Dighub for various security auditing scenarios.

## Table of Contents

- [Basic Scanning](#basic-scanning)
- [Output Formats](#output-formats)
- [Filtering and Prioritization](#filtering-and-prioritization)
- [Performance Optimization](#performance-optimization)
- [Advanced Use Cases](#advanced-use-cases)
- [Automation and CI/CD](#automation-and-cicd)

## Basic Scanning

### Scan a GitHub Organization

```bash
dighub -org mycompany -token ghp_xxxxxxxxxxxxxxxxxxxx
```

### Scan a GitHub User

```bash
dighub -user johndoe -token ghp_xxxxxxxxxxxxxxxxxxxx
```

### Quick Security Audit (High Priority Only)

```bash
dighub -org mycompany -token ghp_xxx -priority high
```

## Output Formats

### Save Results as JSON

```bash
dighub -org mycompany -token ghp_xxx -output json -out-file audit-2024-01-14.json
```

### Generate CSV Report

```bash
dighub -org mycompany -token ghp_xxx -output csv -out-file findings.csv
```

### Create HTML Report

```bash
dighub -org mycompany -token ghp_xxx -output html -out-file report.html
# Open in browser: file://$(pwd)/report.html
```

### Quiet Mode (URLs Only)

```bash
dighub -org mycompany -token ghp_xxx -quiet > urls.txt
```

## Filtering and Prioritization

### Scan for AWS Credentials Only

```bash
dighub -org mycompany -token ghp_xxx -include "AWS"
```

### Scan for Multiple Categories

```bash
dighub -org mycompany -token ghp_xxx -include "AWS,GitHub,SSH,Database"
```

### Exclude Test/Demo Repositories

```bash
dighub -org mycompany -token ghp_xxx -exclude "test,demo,example,sample"
```

### High Priority Findings to HTML

```bash
dighub -org mycompany -token ghp_xxx -priority high -output html
```

### Combine Filters

```bash
dighub -org mycompany \
  -token ghp_xxx \
  -include "AWS,Database" \
  -exclude "backup,log" \
  -priority high \
  -output json
```

## Performance Optimization

DigHub paces requests itself, shared across all workers, so scans stay under the
platform's rate limit without tuning. GitHub's code search API allows 10 requests
per minute, so a full scan of all dorks takes roughly 15 minutes no matter how many
workers you use. If a rate limit is still hit, DigHub waits for the reset and retries.

### Narrow the Scan

Fewer dorks is the main way to scan faster:

```bash
dighub -org mycompany -token ghp_xxx -priority high -include "AWS,GitHub,SSH"
```

### Fetch More Results per Dork

Each dork returns up to 100 results by default. When a dork has more, DigHub
warns and counts it under "Truncated Dorks". Fetch up to 1000:

```bash
dighub -org mycompany -token ghp_xxx -max-pages 10
```

### Self-Hosted Instances

Self-hosted GitLab often has looser limits. Raise the request rate to match:

```bash
dighub -platform gitlab -url https://gitlab.example.com -org platform -token glpat-xxx -rate-limit 120
```

## Advanced Use Cases

### Security Audit with Full Documentation

```bash
dighub -org mycompany \
  -token ghp_xxx \
  -output html \
  -out-file security-audit-$(date +%Y%m%d).html \
  -verbose
```

### Focus on Critical Infrastructure

```bash
dighub -org mycompany \
  -token ghp_xxx \
  -include "AWS,Azure,GCP,Kubernetes,Terraform" \
  -priority high \
  -output json
```

### Payment Gateway Security Check

```bash
dighub -org mycompany \
  -token ghp_xxx \
  -include "Stripe,PayPal,Braintree,Payment" \
  -output csv
```

### CI/CD Secrets Audit

```bash
dighub -org mycompany \
  -token ghp_xxx \
  -include "GitHub,CI/CD,Travis,GitLab" \
  -output json
```

### Webhook Security Scan

```bash
dighub -org mycompany \
  -token ghp_xxx \
  -include "Webhook,Discord,Slack,Teams" \
  -priority medium
```

### Database Credentials Hunt

```bash
dighub -org mycompany \
  -token ghp_xxx \
  -include "Database,Mongo,Postgres,MySQL,Redis" \
  -priority high \
  -verbose
```

## Automation and CI/CD

### Daily Security Scan Script

```bash
#!/bin/bash
# daily-security-scan.sh

DATE=$(date +%Y%m%d)
ORG="mycompany"
TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"
OUTPUT_DIR="./security-reports"

mkdir -p $OUTPUT_DIR

echo "Starting security scan for $ORG..."

# High priority findings
dighub -org $ORG \
  -token $TOKEN \
  -priority high \
  -workers 10 \
  -output json \
  -out-file "$OUTPUT_DIR/high-priority-$DATE.json"

# Full scan
dighub -org $ORG \
  -token $TOKEN \
  -workers 10 \
  -output html \
  -out-file "$OUTPUT_DIR/full-report-$DATE.html"

echo "Scan complete! Reports saved to $OUTPUT_DIR"

# Check if high priority findings exist
FINDINGS=$(jq '.summary.high_priority' "$OUTPUT_DIR/high-priority-$DATE.json")
if [ "$FINDINGS" -gt 0 ]; then
    echo "⚠️  WARNING: $FINDINGS high priority findings detected!"
    # Send alert (email, Slack, etc.)
fi
```

### GitHub Actions Integration

```yaml
name: Security Scan

on:
  schedule:
    - cron: '0 0 * * *'  # Daily at midnight
  workflow_dispatch:

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - name: Install Dighub
        run: |
          go install github.com/ahmetartuc/dighub/v2@latest
          
      - name: Run Security Scan
        env:
          GITHUB_TOKEN: ${{ secrets.DIGHUB_TOKEN }}
        run: |
          dighub -org ${{ github.repository_owner }} \
            -token $GITHUB_TOKEN \
            -priority high \
            -output json \
            -out-file security-report.json
            
      - name: Upload Report
        uses: actions/upload-artifact@v3
        with:
          name: security-report
          path: security-report.json
          
      - name: Check for Critical Findings
        run: |
          HIGH=$(jq '.summary.high_priority' security-report.json)
          if [ "$HIGH" -gt 0 ]; then
            echo "::error::Found $HIGH high priority security issues!"
            exit 1
          fi
```

### Jenkins Pipeline

```groovy
pipeline {
    agent any
    
    environment {
        GITHUB_TOKEN = credentials('github-token')
    }
    
    stages {
        stage('Security Scan') {
            steps {
                sh '''
                    dighub -org mycompany \
                        -token $GITHUB_TOKEN \
                        -priority high \
                        -output json \
                        -out-file security-report.json
                '''
            }
        }
        
        stage('Process Results') {
            steps {
                sh '''
                    FINDINGS=$(jq '.summary.total_matches' security-report.json)
                    echo "Total findings: $FINDINGS"
                    
                    if [ "$FINDINGS" -gt 0 ]; then
                        echo "Security issues detected!"
                    fi
                '''
            }
        }
    }
    
    post {
        always {
            archiveArtifacts artifacts: 'security-report.json'
        }
    }
}
```

### Monitoring Script with Slack Notifications

```bash
#!/bin/bash
# security-monitor.sh

SLACK_WEBHOOK="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
ORG="mycompany"
TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"

# Run scan
dighub -org $ORG \
  -token $TOKEN \
  -priority high \
  -output json \
  -out-file /tmp/scan-results.json \
  -quiet

# Parse results
HIGH=$(jq '.summary.high_priority' /tmp/scan-results.json)
MEDIUM=$(jq '.summary.medium_priority' /tmp/scan-results.json)
TOTAL=$(jq '.summary.total_matches' /tmp/scan-results.json)

# Send to Slack
if [ "$TOTAL" -gt 0 ]; then
    curl -X POST $SLACK_WEBHOOK \
      -H 'Content-Type: application/json' \
      -d "{
        \"text\": \"🔍 Security Scan Results for $ORG\",
        \"attachments\": [{
          \"color\": \"danger\",
          \"fields\": [
            {\"title\": \"High Priority\", \"value\": \"$HIGH\", \"short\": true},
            {\"title\": \"Medium Priority\", \"value\": \"$MEDIUM\", \"short\": true},
            {\"title\": \"Total Findings\", \"value\": \"$TOTAL\", \"short\": false}
          ]
        }]
      }"
fi
```

## Tips and Best Practices

### 1. Start with High Priority

```bash
# Always check high priority first
dighub -org mycompany -token ghp_xxx -priority high
```

### 2. Use Verbose Mode for Debugging

```bash
# See all details including no-match results
dighub -org mycompany -token ghp_xxx -verbose
```

### 3. Export for Analysis

```bash
# Export to CSV for spreadsheet analysis
dighub -org mycompany -token ghp_xxx -output csv
```

### 4. Regular Scheduled Scans

Set up cron jobs for regular scanning:
```bash
# Add to crontab: scan daily at 2 AM
0 2 * * * /path/to/security-scan.sh
```

### 5. Monitor Rate Limits

```bash
# Check your GitHub rate limit
curl -H "Authorization: token ghp_xxx" https://api.github.com/rate_limit
```

## Troubleshooting

### Rate Limit Issues

DigHub waits out rate limits on its own. If you share the token with other tools,
lower DigHub's request rate so they don't compete:

```bash
dighub -org mycompany -token ghp_xxx -rate-limit 5
```

Dorks that still fail are listed at the end of the scan as "Skipped Dorks"
(and under `skipped` in JSON output). Re-run the scan to cover them.

### Token Permission Issues

Ensure your GitHub token has the required scopes:
- `public_repo` for public repositories
- `repo` for private repositories (if needed)

### No Results Found

```bash
# Use verbose mode to see what's being scanned
dighub -org mycompany -token ghp_xxx -verbose
```