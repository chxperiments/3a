<p align="center">
  <img src="docs/assets/logo.png" alt="A3" width="180">
</p>

# A3 / Agnostic Account Assessment

Terminal-based cloud account assessment tool. Discovers resources, maps architecture, evaluates security posture, estimates costs, and generates reports.

Currently supports AWS and OCI via Steampipe.

## Requirements

- Steampipe with the AWS or OCI plugin installed and configured
- Valid cloud credentials (read-only access is sufficient)
- Go 1.24+

## Install

One-line install (Linux and macOS):

```bash
curl -fsSL https://chxmxii.github.io/3a/install.sh | bash
```

Using Go:

```bash
go install github.com/chxmxii/3a/cmd/3a@latest
```

Using Docker/Podman:

```bash
docker pull ghcr.io/chxmxii/3a:latest
docker run --rm -v ~/.aws:/root/.aws -v ~/.3a:/root/.3a ghcr.io/chxmxii/3a assess <profile>
```

From source:

```bash
git clone https://github.com/chxmxii/3a.git
cd 3a
go build -o bin/3a ./cmd/3a/ 
sudo cp bin/3a /usr/local/bin/
```

## Usage

```bash
# Setup Wizard
3a configure

# Run a full assessment
3a assess <profile-name>

# Run without TUI
3a assess <profile-name> --no-tui

# Generate reports
3a report <profile-name> --format markdown
3a report <profile-name> --format json
3a report <profile-name> --format excel -o report.xlsx

# Profile management
3a profiles list
3a profiles add production --provider aws --regions us-east-1,eu-west-1 --aws-profile prod-readonly
```

## What it does

1. Connects to Steampipe and queries 25+ cloud resource types via SQL
2. Reconstructs architecture as two views: Network and Resources.
3. Evaluates 26 security rules against CIS Benchmarks and Well-Architected Framework
4. Fetches real billing data from AWS Cost Explorer (falls back to static estimates)
5. Calculates infrastructure sizing (vCPUs, memory, storage)
6. Generates an adaptive checklist (PASS/FAIL/WARN)
7. Displays everything in an interactive TUI or exports as Markdown/JSON/Excel

## TUI Controls

| Key | Action |
|-----|--------|
| 1-5 | Switch views (Overview, Inventory, Architecture, Findings, Cost) |
| j/k | Scroll up/down |
| Enter | View resource details (type-aware for SGs, Route Tables, IAM Policies) |
| Esc/x | Close detail panel / clear filters |
| r/R | Cycle regions (Inventory) |
| t/T | Cycle resource types (Inventory) |
| n/v | Switch Network/Resource architecture view |
| c/h/m/l | Filter by severity (Findings) |
| q | Quit |

## Configuration

Profiles are stored in `~/.3a/config.yaml`:

```yaml
db_path: ~/.3a/3a.db
steampipe:
  connection_string: postgres://steampipe@localhost:9193/steampipe
profiles:
  - name: production
    provider: aws
    aws_profile: prod-readonly
    regions:
      - us-east-1
      - eu-west-1
```

Assessment data is stored in SQLite at `~/.3a/3a.db`.

## Steampipe Setup

```bash
steampipe plugin install aws
steampipe service start
```

Configure credentials in `~/.steampipe/config/aws.spc`:

```hcl
connection "aws" {
  plugin  = "aws"
  profile = "your-aws-profile"
  regions = ["*"] # All regions
}
```

Or use `3a configure` to set everything up with 3a wizard.

## License

MIT
