---
inclusion: manual
---

# Contributing to 3A

## Project Structure

```
cmd/3a/main.go           Entry point
internal/
  cli/                   Cobra commands (assess, profiles, configure, report)
  config/                YAML config management (~/.3a/config.yaml)
  provider/
    steampipe/           Steampipe discovery (SQL queries against cloud tables)
    provider.go          Provider interfaces and resource type constants
  discovery/             Discovery engine orchestration
  architecture/          Relationship reconstruction rules
  assessment/
    rules/aws/           AWS assessment rules (security, reliability, cost, ops)
    rules/oci/           OCI assessment rules
  sizing/                Infrastructure sizing analysis
  cost/                  Cost estimation (billing API + static catalog fallback)
  checklist/             Adaptive checklist generation
  report/                Report generation (Markdown, JSON, Excel)
  storage/               SQLite storage layer
  tui/                   Bubble Tea TUI (5 views: overview, inventory, arch, findings, cost)
```

## Key Design Decisions

- Discovery uses Steampipe (PostgreSQL queries via pgx) — no direct AWS/OCI SDK calls
- All data persisted to SQLite for offline review
- Assessment rules are Go structs implementing the Rule interface
- TUI uses Bubble Tea with value-receiver Update pattern
- Cost estimation tries AWS Cost Explorer first, falls back to static pricing catalog

## Adding a New Assessment Rule

1. Create a new struct in `internal/assessment/rules/aws/` (or oci/)
2. Implement the `assessment.Rule` interface: ID, Standard, ControlID, Category, AppliesTo, Evaluate
3. Register it in the `AllRules()` function in `rules.go`
4. The rule checks `resource.RawMetadata` fields from Steampipe

## Adding a New Resource Type

1. Add the constant to `internal/provider/provider.go`
2. Add the table mapping in `internal/provider/steampipe/discovery.go` (`awsTableMappings` or `ociTableMappings`)
3. Include `FallbackColumns` for restricted IAM environments
4. Add cost estimation in `internal/cost/estimator.go` if applicable

## Building and Testing

```bash
task build          # Compile to bin/3a
task test           # Run all tests
task check          # fmt + lint + test + build
task setup          # First-time setup
```

## Conventions

- Use `go vet` and `golangci-lint` before committing
- Commit per file when touching multiple concerns
- Use conventional commit messages: feat, fix, refactor, docs
- Keep TUI rendering fast — no network calls in View()
- Assessment rules should be side-effect free (pure evaluation of metadata)
