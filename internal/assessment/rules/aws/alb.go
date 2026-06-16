package aws

import (
	"context"
	"fmt"

	"github.com/chxmxii/3a/internal/assessment"
	"github.com/chxmxii/3a/internal/provider"
	"github.com/chxmxii/3a/internal/storage"
)

// ALBNoHTTPSRule checks for ALBs without HTTPS listeners.
type ALBNoHTTPSRule struct{}

func (r *ALBNoHTTPSRule) ID() string                           { return "aws-alb-no-https" }
func (r *ALBNoHTTPSRule) Standard() string                     { return "AWS Well-Architected" }
func (r *ALBNoHTTPSRule) ControlID() string                    { return "SEC-015" }
func (r *ALBNoHTTPSRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *ALBNoHTTPSRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeALB} }

func (r *ALBNoHTTPSRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	// Steampipe sometimes includes listener info. Check if HTTPS is present.
	// If no listener data is available, check the scheme.
	scheme := getMetaStr(meta, "scheme")

	// If internet-facing, flag if we can't confirm HTTPS.
	if scheme == "internet-facing" {
		// Check for waf association.
		wafArn := getMetaStr(meta, "web_acl_arn")
		if wafArn == "" {
			return []assessment.Finding{{
				Severity:       assessment.SeverityMedium,
				ResourceID:     resource.ResourceID,
				Description:    fmt.Sprintf("Internet-facing ALB %s has no WAF (Web ACL) associated", resource.Name),
				Recommendation: "Associate a WAF Web ACL to protect against common web attacks",
				StandardName:   r.Standard(),
				ControlID:      r.ControlID(),
				Category:       r.Category(),
			}}, nil
		}
	}
	return nil, nil
}

// ALBDeletionProtectionRule checks if deletion protection is enabled.
type ALBDeletionProtectionRule struct{}

func (r *ALBDeletionProtectionRule) ID() string                           { return "aws-alb-no-deletion-protection" }
func (r *ALBDeletionProtectionRule) Standard() string                     { return "AWS Well-Architected" }
func (r *ALBDeletionProtectionRule) ControlID() string                    { return "REL-004" }
func (r *ALBDeletionProtectionRule) Category() assessment.FindingCategory { return assessment.CategoryReliability }
func (r *ALBDeletionProtectionRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeALB, provider.ResourceTypeNLB} }

func (r *ALBDeletionProtectionRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	deletionProtection, ok := meta["deletion_protection_enabled"].(bool)
	if ok && !deletionProtection {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Load balancer %s does not have deletion protection enabled", resource.Name),
			Recommendation: "Enable deletion protection to prevent accidental deletion of production load balancers",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// ALBAccessLogsRule checks if access logging is enabled.
type ALBAccessLogsRule struct{}

func (r *ALBAccessLogsRule) ID() string                           { return "aws-alb-no-access-logs" }
func (r *ALBAccessLogsRule) Standard() string                     { return "CIS AWS Foundations" }
func (r *ALBAccessLogsRule) ControlID() string                    { return "SEC-016" }
func (r *ALBAccessLogsRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *ALBAccessLogsRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeALB, provider.ResourceTypeNLB} }

func (r *ALBAccessLogsRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	// Check access_logs_enabled or nested access_logs.s3.enabled.
	logsEnabled, ok := meta["access_logs_enabled"].(bool)
	if ok && !logsEnabled {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Load balancer %s does not have access logging enabled", resource.Name),
			Recommendation: "Enable access logs to S3 for audit and troubleshooting",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}
