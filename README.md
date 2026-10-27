# DigHub

> **Advanced GitHub Dorking & Secret Hunting Tool**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

DigHub is a CLI tool that scans an organization's or user's repositories on **GitHub** and **GitLab** (including self-hosted GitLab and GitHub Enterprise) for exposed secrets, credentials, and sensitive files. It ships with 140+ dork patterns — covering cloud, database, payment, and AI/LLM provider keys (OpenAI, Anthropic, Gemini, Hugging Face, Groq, and more) — and shows the matched lines for each hit, in terminal, JSON, CSV or HTML output.

![DigHub scanning an organization](assets/screenshot.png)

## Install

```bash
go install github.com/ahmetartuc/dighub@latest
```

Or build from source:

```bash
git clone https://github.com/ahmetartuc/dighub.git
cd dighub && go build -o dighub
```

## Usage

```bash
# Scan an organization
dighub -org <github-org> -token <your_github_pat>

# Scan a user
dighub -user <github-user> -token <your_github_pat>

# High-priority findings only, exported as HTML
dighub -org myorg -token ghp_xxx -priority high -output html -out-file report.html

# Filter to specific categories, fetch up to 300 results per dork
dighub -org myorg -token ghp_xxx -include "AWS,GitHub,SSH" -max-pages 3

# Scan a GitLab group on gitlab.com
dighub -platform gitlab -org mygroup -token glpat-xxx

# Scan a group on a self-hosted GitLab
dighub -platform gitlab -url https://gitlab.example.com -org mygroup -token glpat-xxx
```

Both `-flag` and `--flag` syntax work.

## Options

| Flag | Description |
|------|-------------|
| `-platform, -P` | Platform to search: `github`, `gitlab` (default: `github`) |
| `-url` | Instance URL for self-hosted GitLab or GitHub Enterprise |
| `-org, -o` | Organization (GitHub) or group (GitLab) to scan |
| `-user, -u` | User to scan (alternative to `-org`) |
| `-token, -t` | Personal access token for the platform **(required)** |
| `-output, -f` | Output format: `terminal`, `json`, `csv`, `html` (default: `terminal`) |
| `-out-file, -w` | Output file path (auto-generated if omitted) |
| `-priority, -p` | Filter by priority: `all`, `high`, `medium`, `low` (default: `all`) |
| `-include, -i` | Include only these dork categories (comma-separated) |
| `-exclude, -e` | Exclude these dork categories (comma-separated) |
| `-max-pages, -m` | Result pages per dork, 100 results each, 1–10 (default: `1`) |
| `-rate-limit, -r` | Requests per minute; `0` uses the platform default (GitHub `10`, GitLab `30`) |
| `-workers, -W` | Concurrent workers, 1–20 (default: `5`) |
| `-delay, -d` | Extra delay per worker after each dork, in seconds (default: `0`) |
| `-verbose, -v` | Verbose output |
| `-quiet, -q` | Quiet mode — only show matches |

## Tokens

**GitHub:** Go to [Settings → Developer Settings → Personal Access Tokens](https://github.com/settings/tokens), generate a classic token with the `public_repo` scope, and pass it with `-token`.

**GitLab:** Go to *Preferences → Access Tokens*, create a token with the `read_api` scope, and pass it with `-token`.

## Rate Limits and Coverage

- Requests are paced for the whole scan, not per worker. GitHub's code search API allows 10 requests per minute, so a full scan takes about 15 minutes. If a limit is hit anyway, DigHub waits for the reset and retries.
- A dork that still fails is reported under **Skipped Dorks** in the summary and under `skipped` in JSON output, so a failed search is never mistaken for "nothing found".
- A dork with more results than were fetched is counted under **Truncated Dorks**. Raise `-max-pages` to fetch more (GitHub returns at most 1000 per query).

## GitLab Notes

- Group-wide code search needs advanced search (Elasticsearch or Zoekt) on the instance. When it isn't available, DigHub falls back to searching each project in the group, including subgroups. This works everywhere but takes one request per project per dork, so raise `-rate-limit` on self-hosted instances that allow it.
- `-user` always searches project by project.

## Disclaimer

This tool is intended for **authorized security research and educational purposes only**. Only scan repositories and organizations you have permission to test, and always comply with applicable laws and GitHub's Terms of Service. The authors are not responsible for misuse.

## License

MIT — see [LICENSE](LICENSE). Contributions welcome; see [CONTRIBUTING.md](CONTRIBUTING.md).
