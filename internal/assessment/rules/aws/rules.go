package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/chxmxii/3a/internal/assessment"
	"github.com/chxmxii/3a/internal/provider"
	"github.com/chxmxii/3a/internal/storage"
)

// AllRules returns all AWS assessment rules.
func AllRules() []assessment.Rule {
	return []assessment.Rule{
		// S3
		&S3PublicAccessRule{},
		&S3NoEncryptionRule{},
		// Security Groups
		&SecurityGroupOpenRule{},
		// EBS
		&EBSUnencryptedRule{},
		// RDS
		&RDSPublicRule{},
		&RDSNoMultiAZRule{},
		&RDSNoEncryptionRule{},
		&RDSNoBackupRule{},
		&RDSAutoMinorUpgradeRule{},
		// IAM
		&IAMNoMFARule{},
		// EKS
		&EKSPublicEndpointRule{},
		// EC2
		&EC2PublicIPRule{},
		&EC2StoppedInstanceRule{},
		&EC2NoIMDSv2Rule{},
		// Lambda
		&LambdaNoVPCRule{},
		&LambdaOldRuntimeRule{},
		&LambdaHighMemoryRule{},
		&LambdaNoDeadLetterRule{},
		// VPC / Networking
		&VPCFlowLogsRule{},
		&VPCDefaultSGRule{},
		&SubnetPublicIPAutoAssignRule{},
		// ALB / NLB
		&ALBNoHTTPSRule{},
		&ALBDeletionProtectionRule{},
		&ALBAccessLogsRule{},
	}
}

// S3PublicAccessRule checks for S3 buckets with public access.
type S3PublicAccessRule struct{}

func (r *S3PublicAccessRule) ID() string                            { return "aws-s3-public-access" }
func (r *S3PublicAccessRule) Standard() string                      { return "3A Security Baseline" }
func (r *S3PublicAccessRule) ControlID() string                     { return "SEC-001" }
func (r *S3PublicAccessRule) Category() assessment.FindingCategory  { return assessment.CategorySecurity }
func (r *S3PublicAccessRule) AppliesTo() []provider.ResourceType    { return []provider.ResourceType{provider.ResourceTypeS3Bucket} }

func (r *S3PublicAccessRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	// Check bucket_policy_is_public field from Steampipe.
	if isPublic, ok := meta["bucket_policy_is_public"].(bool); ok && isPublic {
		return []assessment.Finding{{
			Severity:       assessment.SeverityHigh,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("S3 bucket %s has a public bucket policy", resource.Name),
			Recommendation: "Review and restrict the bucket policy to remove public access",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	// Check block public access settings.
	if blockPublicAcls, ok := meta["block_public_acls"].(bool); ok && !blockPublicAcls {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("S3 bucket %s does not block public ACLs", resource.Name),
			Recommendation: "Enable S3 Block Public Access settings on the bucket",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	return nil, nil
}

// SecurityGroupOpenRule checks for security groups open to 0.0.0.0/0 on dangerous ports.
type SecurityGroupOpenRule struct{}

func (r *SecurityGroupOpenRule) ID() string                            { return "aws-sg-open-access" }
func (r *SecurityGroupOpenRule) Standard() string                      { return "3A Security Baseline" }
func (r *SecurityGroupOpenRule) ControlID() string                     { return "SEC-002" }
func (r *SecurityGroupOpenRule) Category() assessment.FindingCategory  { return assessment.CategorySecurity }
func (r *SecurityGroupOpenRule) AppliesTo() []provider.ResourceType    { return []provider.ResourceType{provider.ResourceTypeSecurityGroup} }

func (r *SecurityGroupOpenRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	var findings []assessment.Finding

	// Steampipe stores ingress rules as ip_permissions (array of objects).
	dangerousPorts := map[float64]string{
		22:   "SSH",
		3389: "RDP",
		3306: "MySQL",
		5432: "PostgreSQL",
		1433: "MSSQL",
		6379: "Redis",
		27017: "MongoDB",
	}

	ipPerms, _ := meta["ip_permissions"].([]any)
	for _, perm := range ipPerms {
		permMap, ok := perm.(map[string]any)
		if !ok {
			continue
		}

		fromPort, _ := permMap["from_port"].(float64)
		toPort, _ := permMap["to_port"].(float64)

		// Check IP ranges for 0.0.0.0/0.
		ipRanges, _ := permMap["ip_ranges"].([]any)
		hasOpenCIDR := false
		for _, ipr := range ipRanges {
			iprMap, ok := ipr.(map[string]any)
			if !ok {
				continue
			}
			cidr, _ := iprMap["cidr_ip"].(string)
			if cidr == "0.0.0.0/0" || cidr == "::/0" {
				hasOpenCIDR = true
				break
			}
		}

		if !hasOpenCIDR {
			continue
		}

		// Check if any dangerous port is in the range.
		for port, svc := range dangerousPorts {
			if fromPort <= port && port <= toPort {
				findings = append(findings, assessment.Finding{
					Severity:       assessment.SeverityHigh,
					ResourceID:     resource.ResourceID,
					Description:    fmt.Sprintf("Security group %s allows inbound %s (port %.0f) from 0.0.0.0/0", resource.Name, svc, port),
					Recommendation: fmt.Sprintf("Restrict inbound access on port %.0f to specific IP ranges", port),
					StandardName:   r.Standard(),
					ControlID:      r.ControlID(),
					Category:       r.Category(),
				})
			}
		}

		// If all ports are open (-1 or 0-65535).
		if fromPort == 0 && toPort == 65535 {
			findings = append(findings, assessment.Finding{
				Severity:       assessment.SeverityCritical,
				ResourceID:     resource.ResourceID,
				Description:    fmt.Sprintf("Security group %s allows ALL inbound traffic from 0.0.0.0/0", resource.Name),
				Recommendation: "Restrict inbound access to only required ports and IP ranges",
				StandardName:   r.Standard(),
				ControlID:      r.ControlID(),
				Category:       r.Category(),
			})
		}
	}

	return findings, nil
}

// EBSUnencryptedRule checks for unencrypted EBS volumes.
type EBSUnencryptedRule struct{}

func (r *EBSUnencryptedRule) ID() string                            { return "aws-ebs-unencrypted" }
func (r *EBSUnencryptedRule) Standard() string                      { return "3A Security Baseline" }
func (r *EBSUnencryptedRule) ControlID() string                     { return "SEC-003" }
func (r *EBSUnencryptedRule) Category() assessment.FindingCategory  { return assessment.CategorySecurity }
func (r *EBSUnencryptedRule) AppliesTo() []provider.ResourceType    { return []provider.ResourceType{provider.ResourceTypeEBSVolume} }

func (r *EBSUnencryptedRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	encrypted, ok := meta["encrypted"].(bool)
	if ok && !encrypted {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("EBS volume %s is not encrypted", resource.Name),
			Recommendation: "Enable encryption for EBS volumes. Create a new encrypted volume and migrate data.",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	return nil, nil
}

// RDSPublicRule checks for publicly accessible RDS instances.
type RDSPublicRule struct{}

func (r *RDSPublicRule) ID() string                            { return "aws-rds-public" }
func (r *RDSPublicRule) Standard() string                      { return "3A Security Baseline" }
func (r *RDSPublicRule) ControlID() string                     { return "SEC-004" }
func (r *RDSPublicRule) Category() assessment.FindingCategory  { return assessment.CategorySecurity }
func (r *RDSPublicRule) AppliesTo() []provider.ResourceType    { return []provider.ResourceType{provider.ResourceTypeRDS} }

func (r *RDSPublicRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	publiclyAccessible, ok := meta["publicly_accessible"].(bool)
	if ok && publiclyAccessible {
		return []assessment.Finding{{
			Severity:       assessment.SeverityHigh,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("RDS instance %s is publicly accessible", resource.Name),
			Recommendation: "Disable public accessibility for the RDS instance and use VPC connectivity",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	return nil, nil
}

// IAMNoMFARule checks for IAM users without MFA enabled.
type IAMNoMFARule struct{}

func (r *IAMNoMFARule) ID() string                            { return "aws-iam-no-mfa" }
func (r *IAMNoMFARule) Standard() string                      { return "3A Security Baseline" }
func (r *IAMNoMFARule) ControlID() string                     { return "SEC-005" }
func (r *IAMNoMFARule) Category() assessment.FindingCategory  { return assessment.CategorySecurity }
func (r *IAMNoMFARule) AppliesTo() []provider.ResourceType    { return []provider.ResourceType{provider.ResourceTypeIAMUser} }

func (r *IAMNoMFARule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	// Steampipe aws_iam_user has mfa_enabled field.
	mfaEnabled, ok := meta["mfa_enabled"].(bool)
	if ok && !mfaEnabled {
		return []assessment.Finding{{
			Severity:       assessment.SeverityHigh,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("IAM user %s does not have MFA enabled", resource.Name),
			Recommendation: "Enable MFA for all IAM users, especially those with console access",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	// Also check mfa_devices count.
	if devices, ok := meta["mfa_devices"].([]any); ok && len(devices) == 0 {
		return []assessment.Finding{{
			Severity:       assessment.SeverityHigh,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("IAM user %s has no MFA devices configured", resource.Name),
			Recommendation: "Enable MFA for all IAM users, especially those with console access",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	return nil, nil
}

// EKSPublicEndpointRule checks for EKS clusters with public API endpoints.
type EKSPublicEndpointRule struct{}

func (r *EKSPublicEndpointRule) ID() string                            { return "aws-eks-public-endpoint" }
func (r *EKSPublicEndpointRule) Standard() string                      { return "3A Security Baseline" }
func (r *EKSPublicEndpointRule) ControlID() string                     { return "SEC-006" }
func (r *EKSPublicEndpointRule) Category() assessment.FindingCategory  { return assessment.CategorySecurity }
func (r *EKSPublicEndpointRule) AppliesTo() []provider.ResourceType    { return []provider.ResourceType{provider.ResourceTypeEKSCluster} }

func (r *EKSPublicEndpointRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	// Steampipe stores this in resources_vpc_config -> endpoint_public_access.
	if vpcConfig, ok := meta["resources_vpc_config"].(map[string]any); ok {
		if publicAccess, ok := vpcConfig["endpoint_public_access"].(bool); ok && publicAccess {
			// Check if private access is also enabled (less severe).
			privateAccess, _ := vpcConfig["endpoint_private_access"].(bool)
			severity := assessment.SeverityHigh
			if privateAccess {
				severity = assessment.SeverityMedium
			}
			return []assessment.Finding{{
				Severity:       severity,
				ResourceID:     resource.ResourceID,
				Description:    fmt.Sprintf("EKS cluster %s has public API endpoint enabled", resource.Name),
				Recommendation: "Disable public endpoint access and use private endpoint with VPN/DirectConnect",
				StandardName:   r.Standard(),
				ControlID:      r.ControlID(),
				Category:       r.Category(),
			}}, nil
		}
	}

	// Also check top-level endpoint_public_access (Steampipe flattened).
	if pub, ok := meta["endpoint_public_access"].(bool); ok && pub {
		return []assessment.Finding{{
			Severity:       assessment.SeverityHigh,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("EKS cluster %s has public API endpoint enabled", resource.Name),
			Recommendation: "Disable public endpoint access and use private endpoint with VPN/DirectConnect",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	return nil, nil
}

// S3NoEncryptionRule checks for S3 buckets without default encryption.
type S3NoEncryptionRule struct{}

func (r *S3NoEncryptionRule) ID() string                            { return "aws-s3-no-encryption" }
func (r *S3NoEncryptionRule) Standard() string                      { return "3A Security Baseline" }
func (r *S3NoEncryptionRule) ControlID() string                     { return "SEC-007" }
func (r *S3NoEncryptionRule) Category() assessment.FindingCategory  { return assessment.CategorySecurity }
func (r *S3NoEncryptionRule) AppliesTo() []provider.ResourceType    { return []provider.ResourceType{provider.ResourceTypeS3Bucket} }

func (r *S3NoEncryptionRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	// Steampipe: server_side_encryption_configuration is null if not configured.
	encConfig := meta["server_side_encryption_configuration"]
	if encConfig == nil {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("S3 bucket %s does not have default encryption configured", resource.Name),
			Recommendation: "Enable default encryption (SSE-S3 or SSE-KMS) on the bucket",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	// Check if it's an empty string or "null".
	if s, ok := encConfig.(string); ok && (s == "" || strings.ToLower(s) == "null") {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("S3 bucket %s does not have default encryption configured", resource.Name),
			Recommendation: "Enable default encryption (SSE-S3 or SSE-KMS) on the bucket",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}

	return nil, nil
}
