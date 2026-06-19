<p align="center">
  <img src="docs/assets/logo.png" alt="3A" width="180">
</p>

# 3A / Agnostic Account Assessment

Terminal-based cloud account assessment tool. Discovers resources, maps architecture, evaluates security posture, estimates costs, and generates reports.

Supports AWS and OCI via Steampipe.

## Requirements

- Go 1.25+
- Steampipe with the AWS or OCI plugin installed and configured
- Valid cloud credentials (read-only access is sufficient)

## Install

One-line install (Linux and macOS):

```bash
curl -fsSL https://chxmxii.github.io/3a/install.sh | bash
```

Or build from source:

```bash
git clone https://github.com/chxmxii/3a.git
cd 3a
go build -o bin/3a ./cmd/3a/
sudo cp bin/3a /usr/local/bin/
```

Uisng docker/podmna:

```bash
docker pull docker pull ghcr.io/chxmxii/3a:latest
```

## Usage

```bash
# Add a profile
3a profiles add

# Configure Wizard
3a configure

# Run assessment (opens TUI when done)
3a assess <profile-name>

# Run without TUI
3a assess <profile-name> --no-tui

# Generate reports (markdown + JSON + excel)
3a report <profile-name>

# List profiles
3a profiles list

```

## What it does

1. Connects to Steampipe and queries cloud resources.
2. Reconstructs architecture relationships (VPC-to-Subnet, EC2-to-SecurityGroup, etc.)
3. Evaluates resources against CIS Benchmarks and Well-Architected Framework rules
4. Calculates infrastructure sizing (vCPUs, memory, storage)
5. Estimates monthly costs with top cost drivers and idle/oversized detection
6. Generates an adaptive checklist (PASS/FAIL/WARN)
7. Displays results in an interactive TUI or exports as Markdown/JSON reports

## TUI Controls

| Key | Action |
|-----|--------|
| 1-5 | Switch views (Overview, Inventory, Architecture, Findings, Cost) |
| j/k | Scroll up/down |
| r/R | Cycle regions (Inventory) |
| t | Cycle resource types (Inventory) |
| c/h/m/l | Filter by severity (Findings) |
| x | Clear filters |
| q | Quit |

## Configuration

Profiles are stored in `~/.3a/config.yaml`:

```yaml
db_path: ~/.3a/assessments.db
profiles:
  - name: production
    provider: aws
    credentials:
      type: profile
      profile_name: prod-readonly
    regions:
      - us-east-1
      - eu-west-1
```

Assessment data is stored in SQLite at `~/.3a/assessments.db`.

## Steampipe Setup

```bash
# Install AWS plugin
steampipe plugin install aws

# Install OCI plugin
steampipe plugin install oci

steampipe service start
```

Configure credentials in `~/.steampipe/config/aws.spc` ():

```hcl
connection "aws" {
  plugin  = "aws"
  profile = "your-aws-profile"
  regions = ["us-east-1", "eu-west-1"]
}
```

## License

MIT
