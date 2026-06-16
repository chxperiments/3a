package aws

import (
	"context"
	"fmt"

	"github.com/chxmxii/3a/internal/assessment"
	"github.com/chxmxii/3a/internal/provider"
	"github.com/chxmxii/3a/internal/storage"
)

// LambdaNoVPCRule checks for Lambda functions not attached to a VPC.
type LambdaNoVPCRule struct{}

func (r *LambdaNoVPCRule) ID() string                           { return "aws-lambda-no-vpc" }
func (r *LambdaNoVPCRule) Standard() string                     { return "AWS Well-Architected" }
func (r *LambdaNoVPCRule) ControlID() string                    { return "SEC-008" }
func (r *LambdaNoVPCRule) Category() assessment.FindingCategory { return assessment.CategorySecurity }
func (r *LambdaNoVPCRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeLambda} }

func (r *LambdaNoVPCRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	vpcID := getMetaStr(meta, "vpc_id")
	if vpcID == "" {
		// Check nested vpc_config.
		if vpcCfg, ok := meta["vpc_config"].(map[string]any); ok {
			vpcID, _ = vpcCfg["vpc_id"].(string)
		}
	}

	if vpcID == "" {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Lambda function %s is not attached to a VPC", resource.Name),
			Recommendation: "Attach the function to a VPC if it accesses private resources",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// LambdaOldRuntimeRule checks for Lambda functions using deprecated runtimes.
type LambdaOldRuntimeRule struct{}

func (r *LambdaOldRuntimeRule) ID() string                           { return "aws-lambda-old-runtime" }
func (r *LambdaOldRuntimeRule) Standard() string                     { return "AWS Well-Architected" }
func (r *LambdaOldRuntimeRule) ControlID() string                    { return "OPS-001" }
func (r *LambdaOldRuntimeRule) Category() assessment.FindingCategory { return assessment.CategoryOperationalExcellence }
func (r *LambdaOldRuntimeRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeLambda} }

func (r *LambdaOldRuntimeRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	runtime := getMetaStr(meta, "runtime")

	deprecated := map[string]bool{
		"python2.7":    true,
		"python3.6":    true,
		"python3.7":    true,
		"nodejs10.x":   true,
		"nodejs12.x":   true,
		"nodejs14.x":   true,
		"dotnetcore2.1": true,
		"dotnetcore3.1": true,
		"ruby2.5":      true,
		"ruby2.7":      true,
		"java8":        true,
		"go1.x":        true,
	}

	if deprecated[runtime] {
		return []assessment.Finding{{
			Severity:       assessment.SeverityMedium,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Lambda %s uses deprecated runtime %s", resource.Name, runtime),
			Recommendation: "Upgrade to a supported runtime version to receive security patches",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// LambdaHighMemoryRule checks for over-provisioned Lambda functions.
type LambdaHighMemoryRule struct{}

func (r *LambdaHighMemoryRule) ID() string                           { return "aws-lambda-high-memory" }
func (r *LambdaHighMemoryRule) Standard() string                     { return "AWS Well-Architected" }
func (r *LambdaHighMemoryRule) ControlID() string                    { return "COST-001" }
func (r *LambdaHighMemoryRule) Category() assessment.FindingCategory { return assessment.CategoryCostOptimization }
func (r *LambdaHighMemoryRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeLambda} }

func (r *LambdaHighMemoryRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata
	memorySize, ok := meta["memory_size"].(float64)
	if !ok {
		return nil, nil
	}

	if memorySize >= 3008 {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Lambda %s has %.0f MB memory allocated (potentially over-provisioned)", resource.Name, memorySize),
			Recommendation: "Review actual memory usage and reduce allocation if peak usage is well below configured memory",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

// LambdaNoDeadLetterRule checks for Lambda without DLQ configured.
type LambdaNoDeadLetterRule struct{}

func (r *LambdaNoDeadLetterRule) ID() string                           { return "aws-lambda-no-dlq" }
func (r *LambdaNoDeadLetterRule) Standard() string                     { return "AWS Well-Architected" }
func (r *LambdaNoDeadLetterRule) ControlID() string                    { return "REL-001" }
func (r *LambdaNoDeadLetterRule) Category() assessment.FindingCategory { return assessment.CategoryReliability }
func (r *LambdaNoDeadLetterRule) AppliesTo() []provider.ResourceType   { return []provider.ResourceType{provider.ResourceTypeLambda} }

func (r *LambdaNoDeadLetterRule) Evaluate(_ context.Context, resource storage.Resource) ([]assessment.Finding, error) {
	meta := resource.RawMetadata

	dlqArn := getMetaStr(meta, "dead_letter_config_target_arn")
	if dlqArn == "" {
		if dlq, ok := meta["dead_letter_config"].(map[string]any); ok {
			dlqArn, _ = dlq["target_arn"].(string)
		}
	}

	if dlqArn == "" {
		return []assessment.Finding{{
			Severity:       assessment.SeverityLow,
			ResourceID:     resource.ResourceID,
			Description:    fmt.Sprintf("Lambda %s has no dead letter queue configured", resource.Name),
			Recommendation: "Configure a DLQ (SQS or SNS) to capture failed async invocations",
			StandardName:   r.Standard(),
			ControlID:      r.ControlID(),
			Category:       r.Category(),
		}}, nil
	}
	return nil, nil
}

func getMetaStr(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	v, ok := meta[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
