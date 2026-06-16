package aws

import (
	"context"
	"fmt"

	"github.com/chxmxii/3a/internal/assessment"
	"github.com/chxmxii/3a/internal/provider"
	"github.com/chxmxii/3a/internal/storage"
)

// EC2PublicIPRule checks for EC2 instances with public IPs.
type EC2PublicIPRule struct{}

func (r *EC2PublicIPRule) ID() string                           { return "aws-ec2-public-ip" }
func (r *EC2PublicIPRule) Standard() string                     { return "CIS AWS Foundations" }
func (r *EC2PublicIPRule) ControlID() string                    { return "SEC-009" }
func (r *EC2PublicIPRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *EC2PublicIPRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeEC2Instance} }

func (r *EC2PublicIPRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	publicIP := getMetaStr(meta, "public_ip_address")

	if publicIP != "" {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("EC2 instance %s has a public IP (%s)", resource.Name, publicIP),
			Recommendation: "Use a load balancer or NAT gateway instead of direct public IP assignment",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// EC2StoppedInstanceRule checks for instances that have been stopped (wasting money on EBS).
type EC2StoppedInstanceRule struct{}

func (r *EC2StoppedInstanceRule) ID() string                           { return "aws-ec2-stopped" }
func (r *EC2StoppedInstanceRule) Standard() string                     { return "AWS Well-Architected" }
func (r *EC2StoppedInstanceRule) ControlID() string                    { return "COST-002" }
func (r *EC2StoppedInstanceRule) Category() assessment.FindingCategory { return assessment.CategoryCostOptimization }
func (r *EC2StoppedInstanceRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeEC2Instance} }

func (r *EC2StoppedInstanceRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	state := getMetaStr(meta, "instance_state")
	if state == "" {
		state = getMetaStr(meta, "state")
	}

	if state == "stopped" {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("EC2 instance %s is stopped (EBS volumes still incur charges)", resource.Name),
			Recommendation: "Terminate the instance if no longer needed, or snapshot and delete EBS volumes",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// EC2NoIMDSv2Rule checks if instance metadata service v2 is not enforced.
type EC2NoIMDSv2Rule struct{}

func (r *EC2NoIMDSv2Rule) ID() string                           { return "aws-ec2-no-imdsv2" }
func (r *EC2NoIMDSv2Rule) Standard() string                     { return "CIS AWS Foundations" }
func (r *EC2NoIMDSv2Rule) ControlID() string                    { return "SEC-010" }
func (r *EC2NoIMDSv2Rule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *EC2NoIMDSv2Rule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeEC2Instance} }

func (r *EC2NoIMDSv2Rule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	httpTokens := getMetaStr(meta, "metadata_options_http_tokens")
	if httpTokens == "" {
		if opts, ok := meta["metadata_options"].(map[string]any); ok {
			httpTokens, _ = opts["http_tokens"].(string)
		}
	}

	// "required" means IMDSv2 is enforced. "optional" means v1 is still allowed.
	if httpTokens == "optional" || httpTokens == "" {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("EC2 instance %s does not enforce IMDSv2", resource.Name),
			Recommendation: "Set HttpTokens to 'required' to enforce IMDSv2 and prevent SSRF-based credential theft",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}
