package steampipe

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/chxmxii/a3/internal/provider"
)

// SteampipeProvider implements the provider.Provider interface using Steampipe's
// PostgreSQL-compatible endpoint for resource discovery.
type SteampipeProvider struct {
	pool         *pgxpool.Pool
	connString   string
	providerType string // "aws" or "oci"
}

// NewSteampipeProvider creates a new Steampipe-backed provider.
func NewSteampipeProvider(connString string, providerType string) (*SteampipeProvider, error) {
	if connString == "" {
		connString = "postgres://steampipe@localhost:9193/steampipe"
	}
	return &SteampipeProvider{
		connString:   connString,
		providerType: providerType,
	}, nil
}

// Name returns the provider type identifier.
func (s *SteampipeProvider) Name() string { return s.providerType }

// Authenticate connects to the Steampipe database and verifies connectivity.
func (s *SteampipeProvider) Authenticate(ctx context.Context) error {
	pool, err := pgxpool.New(ctx, s.connString)
	if err != nil {
		return fmt.Errorf("failed to connect to steampipe: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("steampipe connection test failed: %w", err)
	}
	s.pool = pool
	return nil
}

// Discoverer returns the Steampipe-based resource discoverer.
func (s *SteampipeProvider) Discoverer() provider.Discoverer {
	return &SteampipeDiscoverer{pool: s.pool, providerType: s.providerType}
}

// MetricsClient returns nil — Steampipe discovery doesn't provide metrics.
func (s *SteampipeProvider) MetricsClient() provider.MetricsClient { return nil }

// PricingClient returns nil — pricing is handled separately.
func (s *SteampipeProvider) PricingClient() provider.PricingClient { return nil }

// ValidateProfile checks that Steampipe is reachable and the provider plugin is
// installed for the configured provider type.
//
// It deliberately does NOT require that the configured credentials have list/read
// permissions: a read-only role that is denied on the canary table (e.g. an AWS
// SSO role without iam:ListAccountAliases) is still a perfectly valid target —
// discovery handles per-table permission denials gracefully and reports whatever
// the role can read. Such permission errors are returned as a non-fatal warning
// (warn != "", err == nil) so the caller can surface them and continue.
//
// A non-nil error is returned only for conditions that make the assessment
// impossible: an unestablished connection, the plugin not being installed, or
// credentials that are missing/expired (as opposed to merely under-privileged).
func (s *SteampipeProvider) ValidateProfile(ctx context.Context) (warn string, err error) {
	if s.pool == nil {
		return "", fmt.Errorf("steampipe connection not established — call Authenticate first")
	}

	// Pick a lightweight "canary" table per provider to test connectivity.
	var canaryTable string
	switch s.providerType {
	case "aws":
		canaryTable = "aws_account"
	case "oci":
		canaryTable = "oci_identity_compartment"
	default:
		return "", fmt.Errorf("unsupported provider type: %s", s.providerType)
	}

	// Check if the table exists — i.e. the plugin is installed and configured.
	var tableExists bool
	qErr := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = $1
		)
	`, canaryTable).Scan(&tableExists)
	if qErr != nil {
		return "", fmt.Errorf("failed to check steampipe tables: %w\n\nIs the %s plugin installed? Run:\n  steampipe plugin install %s", qErr, s.providerType, s.providerType)
	}
	if !tableExists {
		return "", fmt.Errorf("steampipe table %q not found\n\nThe %s plugin may not be installed or configured. Run:\n  steampipe plugin install %s\n  steampipe plugin list", canaryTable, s.providerType, s.providerType)
	}

	// Probe the canary to confirm credentials are wired up. A success or a
	// permission denial both mean the connection is reachable; only a missing/
	// expired credential is fatal.
	var rowCount int
	query := fmt.Sprintf("SELECT count(*) FROM %s", canaryTable)
	probeErr := s.pool.QueryRow(ctx, query).Scan(&rowCount)
	if probeErr != nil {
		switch {
		case isPermissionDenied(probeErr):
			return fmt.Sprintf("credentials are valid but lack permission to read %q — discovery will only cover resources the role is allowed to list", canaryTable), nil
		case isMissingCredentials(probeErr):
			return "", fmt.Errorf("steampipe query to %s failed: %w\n\nCloud credentials appear to be missing or expired.\nCheck your Steampipe connection config and re-authenticate:\n  cat ~/.steampipe/config/%s.spc", canaryTable, probeErr, s.providerType)
		default:
			// Unknown failure: don't block the assessment — discovery's
			// per-table handling will surface what is and isn't reachable.
			return fmt.Sprintf("could not fully validate %q (%v) — continuing; discovery will report per-resource access", canaryTable, probeErr), nil
		}
	}

	return "", nil
}

// isPermissionDenied reports whether err is an authorization failure from the
// cloud provider (the role is valid but not allowed to perform the action),
// as opposed to a credential or connectivity problem.
func isPermissionDenied(err error) bool {
	msg := err.Error()
	for _, needle := range []string{
		"AccessDenied",          // AWS S3/IAM
		"UnauthorizedOperation", // AWS EC2
		"AuthorizationError",    // AWS misc
		"not authorized to perform",
		"403",
		"NotAuthorizedOrNotFound", // OCI
		"insufficient permission",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// isMissingCredentials reports whether err indicates that no usable credentials
// are configured (missing, expired, or unresolvable), which is fatal.
func isMissingCredentials(err error) bool {
	msg := err.Error()
	for _, needle := range []string{
		"ExpiredToken",
		"InvalidClientTokenId",
		"failed to refresh cached credentials",
		"no EC2 IMDS role found",
		"NoCredentialProviders",
		"could not find",
		"SSO session",
		"token has expired",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// Close releases the connection pool.
func (s *SteampipeProvider) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Pool returns the underlying connection pool for direct queries.
func (s *SteampipeProvider) Pool() *pgxpool.Pool {
	return s.pool
}
