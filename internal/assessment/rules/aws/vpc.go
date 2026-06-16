package aws

import (
	"context"
	"fmt"

	"github.com/chxmxii/3a/internal/assessment"
	"github.com/chxmxii/3a/internal/provider"
	"github.com/chxmxii/3a/internal/storage"
)

// VPCFlowLogsRule checks if VPC flow logs are enabled.
type VPCFlowLogsRule struct{}

func (r *VPCFlowLogsRule) ID() string                           { return "aws-vpc-no-flow-logs" }
func (r *VPCFlowLogsRule) Standard() string                     { return "CIS AWS Foundations" }
func (r *VPCFlowLogsRule) ControlID() string                    { return "SEC-012" }
func (r *VPCFlowLogsRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *VPCFlowLogsRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeVPC} }

func (r *VPCFlowLogsRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	// Steampipe: flow_logs is an array. Empty or nil means no flow logs.
	flowLogs, ok := meta["flow_logs"].([]any)
	if !ok || len(flowLogs) == 0 {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("VPC %s does not have flow logs enabled", resource.Name),
			Recommendation: "Enable VPC Flow Logs to capture network traffic for security analysis",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// VPCDefaultSGRule checks if the default security group allows traffic.
type VPCDefaultSGRule struct{}

func (r *VPCDefaultSGRule) ID() string                           { return "aws-vpc-default-sg" }
func (r *VPCDefaultSGRule) Standard() string                     { return "CIS AWS Foundations" }
func (r *VPCDefaultSGRule) ControlID() string                    { return "SEC-013" }
func (r *VPCDefaultSGRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *VPCDefaultSGRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeSecurityGroup} }

func (r *VPCDefaultSGRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	groupName := getMetaStr(meta, "group_name")
	if groupName != "default" {
		return nil, nil
	}

	// Check if default SG has any ingress rules.
	ipPerms, _ := meta["ip_permissions"].([]any)
	if len(ipPerms) > 0 {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Default security group in VPC has inbound rules configured"),
			Recommendation: "Remove all inbound/outbound rules from the default security group. Use custom SGs instead.",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// SubnetPublicIPAutoAssignRule checks subnets that auto-assign public IPs.
type SubnetPublicIPAutoAssignRule struct{}

func (r *SubnetPublicIPAutoAssignRule) ID() string                           { return "aws-subnet-public-ip" }
func (r *SubnetPublicIPAutoAssignRule) Standard() string                     { return "AWS Well-Architected" }
func (r *SubnetPublicIPAutoAssignRule) ControlID() string                    { return "SEC-014" }
func (r *SubnetPublicIPAutoAssignRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *SubnetPublicIPAutoAssignRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeSubnet} }

func (r *SubnetPublicIPAutoAssignRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	autoAssign, ok := meta["map_public_ip_on_launch"].(bool)
	if ok && autoAssign {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Subnet %s auto-assigns public IPs to instances", resource.Name),
			Recommendation: "Disable auto-assign public IP unless the subnet is intentionally public-facing",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}
