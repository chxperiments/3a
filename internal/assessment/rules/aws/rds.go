package aws

import (
	"context"
	"fmt"

	"github.com/chxmxii/3a/internal/assessment"
	"github.com/chxmxii/3a/internal/provider"
	"github.com/chxmxii/3a/internal/storage"
)

// RDSNoMultiAZRule checks for RDS instances without Multi-AZ.
type RDSNoMultiAZRule struct{}

func (r *RDSNoMultiAZRule) ID() string                           { return "aws-rds-no-multi-az" }
func (r *RDSNoMultiAZRule) Standard() string                     { return "AWS Well-Architected" }
func (r *RDSNoMultiAZRule) ControlID() string                    { return "REL-002" }
func (r *RDSNoMultiAZRule) Category() assessment.FindingCategory { return assessment.CategoryReliability }
func (r *RDSNoMultiAZRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeRDS} }

func (r *RDSNoMultiAZRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	multiAZ, ok := meta["multi_az"].(bool)
	if ok && !multiAZ {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("RDS instance %s is not Multi-AZ", resource.Name),
			Recommendation: "Enable Multi-AZ for production databases to ensure automatic failover",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// RDSNoEncryptionRule checks for unencrypted RDS instances.
type RDSNoEncryptionRule struct{}

func (r *RDSNoEncryptionRule) ID() string                           { return "aws-rds-no-encryption" }
func (r *RDSNoEncryptionRule) Standard() string                     { return "CIS AWS Foundations" }
func (r *RDSNoEncryptionRule) ControlID() string                    { return "SEC-011" }
func (r *RDSNoEncryptionRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *RDSNoEncryptionRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeRDS} }

func (r *RDSNoEncryptionRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	encrypted, ok := meta["storage_encrypted"].(bool)
	if ok && !encrypted {
		return []assessment.Finding{{
			Severity:       assessment.SeverityHigh,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("RDS instance %s storage is not encrypted", resource.Name),
			Recommendation: "Enable encryption at rest. Requires creating a new encrypted instance and migrating data.",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// RDSNoBackupRule checks for RDS instances with no automated backups.
type RDSNoBackupRule struct{}

func (r *RDSNoBackupRule) ID() string                           { return "aws-rds-no-backup" }
func (r *RDSNoBackupRule) Standard() string                     { return "AWS Well-Architected" }
func (r *RDSNoBackupRule) ControlID() string                    { return "REL-003" }
func (r *RDSNoBackupRule) Category() assessment.FindingCategory { return assessment.CategoryReliability }
func (r *RDSNoBackupRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeRDS} }

func (r *RDSNoBackupRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	retention, ok := meta["backup_retention_period"].(float64)
	if ok && retention == 0 {
		return []assessment.Finding{{
			Severity:       assessment.SeverityHigh,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("RDS instance %s has automated backups disabled (retention = 0)", resource.Name),
			Recommendation: "Enable automated backups with at least 7 days retention",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// RDSAutoMinorUpgradeRule checks if auto minor version upgrade is disabled.
type RDSAutoMinorUpgradeRule struct{}

func (r *RDSAutoMinorUpgradeRule) ID() string                           { return "aws-rds-no-auto-upgrade" }
func (r *RDSAutoMinorUpgradeRule) Standard() string                     { return "AWS Well-Architected" }
func (r *RDSAutoMinorUpgradeRule) ControlID() string                    { return "OPS-002" }
func (r *RDSAutoMinorUpgradeRule) Category() assessment.FindingCategory { return assessment.CategoryOperationalExcellence }
func (r *RDSAutoMinorUpgradeRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeRDS} }

func (r *RDSAutoMinorUpgradeRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	autoUpgrade, ok := meta["auto_minor_version_upgrade"].(bool)
	if ok && !autoUpgrade {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("RDS instance %s has auto minor version upgrade disabled", resource.Name),
			Recommendation: "Enable auto minor version upgrades to receive security patches automatically",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}
