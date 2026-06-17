package steampipe

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/chxmxii/3a/internal/provider"
)

// tableMapping maps a Steampipe table name to the internal ResourceType.
type tableMapping struct {
	Table           string
	ResourceType    provider.ResourceType
	IDColumn        string   // column used as resource ID (arn, id, etc.)
	NameColumn      string   // column used as display name
	RegionColumn    string   // column for region (empty for global)
	FallbackColumns []string // minimal columns to query when SELECT * fails (nil = no fallback)
}

// SteampipeDiscoverer queries Steampipe tables to discover cloud resources.
type SteampipeDiscoverer struct {
	pool         *pgxpool.Pool
	providerType string
}

// SupportedResourceTypes returns all resource types this discoverer can enumerate.
func (d *SteampipeDiscoverer) SupportedResourceTypes() []provider.ResourceType {
	types := make([]provider.ResourceType, 0, len(d.tableMappings()))
	for _, m := range d.tableMappings() {
		types = append(types, m.ResourceType)
	}
	return types
}

// DiscoverResources queries all configured Steampipe tables and streams results.
// The regions parameter is ignored — Steampipe handles multi-region discovery via its own config.
func (d *SteampipeDiscoverer) DiscoverResources(ctx context.Context, regions []string, results chan<- provider.DiscoveredResource) error {
	for _, mapping := range d.tableMappings() {
		if err := d.queryTable(ctx, mapping, results); err != nil {
			// Classify the error for cleaner output.
			errStr := err.Error()
			switch {
			case contains(errStr, "does not exist"):
				log.Printf("[steampipe] ⚠ table %s not available (skipped)", mapping.Table)
			case contains(errStr, "AccessDenied") || contains(errStr, "UnauthorizedOperation") || contains(errStr, "AccessDeniedException"):
				log.Printf("[steampipe] 🔒 %s: insufficient permissions (skipped)", mapping.Table)
			default:
				log.Printf("[steampipe] ❌ %s: %v", mapping.Table, err)
			}
		}
	}
	return nil
}

// queryTable runs SELECT * against a Steampipe table and converts rows to DiscoveredResource.
// If SELECT * fails and FallbackColumns are defined, retries with only those columns.
func (d *SteampipeDiscoverer) queryTable(ctx context.Context, mapping tableMapping, results chan<- provider.DiscoveredResource) error {
	query := fmt.Sprintf("SELECT * FROM %s", mapping.Table)
	rows, err := d.pool.Query(ctx, query)
	if err != nil {
		// Try fallback with minimal columns if available.
		if mapping.FallbackColumns != nil {
			return d.queryTableFallback(ctx, mapping, results)
		}
		return fmt.Errorf("querying %s: %w", mapping.Table, err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	colNames := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		colNames[i] = string(fd.Name)
	}

	count := 0
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			// Row-level errors — skip silently.
			continue
		}

		// Build a metadata map from all columns.
		metadata := make(map[string]any, len(colNames))
		for i, col := range colNames {
			metadata[col] = values[i]
		}

		res := d.buildResource(mapping, metadata)
		if res != nil {
			results <- *res
			count++
		}
	}

	if err := rows.Err(); err != nil {
		// If we got zero rows due to an error, try fallback.
		if count == 0 && mapping.FallbackColumns != nil {
			return d.queryTableFallback(ctx, mapping, results)
		}
		return err
	}

	return nil
}

// queryTableFallback queries only the minimal columns needed for inventory.
func (d *SteampipeDiscoverer) queryTableFallback(ctx context.Context, mapping tableMapping, results chan<- provider.DiscoveredResource) error {
	cols := strings.Join(mapping.FallbackColumns, ", ")
	query := fmt.Sprintf("SELECT %s FROM %s", cols, mapping.Table)
	rows, err := d.pool.Query(ctx, query)
	if err != nil {
		// Last resort: try just the ID and name columns.
		return d.queryTableMinimal(ctx, mapping, results)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	colNames := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		colNames[i] = string(fd.Name)
	}

	count := 0
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			continue
		}

		metadata := make(map[string]any, len(colNames))
		for i, col := range colNames {
			metadata[col] = values[i]
		}

		res := d.buildResource(mapping, metadata)
		if res != nil {
			results <- *res
			count++
		}
	}

	if err := rows.Err(); err != nil && count == 0 {
		return d.queryTableMinimal(ctx, mapping, results)
	}

	return nil
}

// queryTableMinimal tries the absolute minimum: just ID, name, and region.
func (d *SteampipeDiscoverer) queryTableMinimal(ctx context.Context, mapping tableMapping, results chan<- provider.DiscoveredResource) error {
	cols := []string{mapping.IDColumn}
	if mapping.NameColumn != "" && mapping.NameColumn != mapping.IDColumn {
		cols = append(cols, mapping.NameColumn)
	}
	if mapping.RegionColumn != "" {
		cols = append(cols, mapping.RegionColumn)
	}

	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), mapping.Table)
	rows, err := d.pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("minimal query %s: %w", mapping.Table, err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	colNames := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		colNames[i] = string(fd.Name)
	}

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			continue
		}

		metadata := make(map[string]any, len(colNames))
		for i, col := range colNames {
			metadata[col] = values[i]
		}

		res := d.buildResource(mapping, metadata)
		if res != nil {
			results <- *res
		}
	}

	return rows.Err()
}

// buildResource converts a metadata map into a DiscoveredResource.
func (d *SteampipeDiscoverer) buildResource(mapping tableMapping, metadata map[string]any) *provider.DiscoveredResource {
	// Extract resource ID.
	resourceID := getStringFromMap(metadata, mapping.IDColumn)
	if resourceID == "" {
		for _, col := range []string{"arn", "id", "resource_id"} {
			resourceID = getStringFromMap(metadata, col)
			if resourceID != "" {
				break
			}
		}
	}
	if resourceID == "" {
		return nil
	}

	// Extract name.
	name := getStringFromMap(metadata, mapping.NameColumn)
	if name == "" {
		name = getStringFromMap(metadata, "title")
	}

	// Extract region.
	region := getStringFromMap(metadata, mapping.RegionColumn)
	if region == "" {
		region = getStringFromMap(metadata, "region")
	}
	if region == "" {
		region = "global"
	}

	// Extract tags.
	tags := extractTags(metadata)

	return &provider.DiscoveredResource{
		ProviderType: d.providerType,
		ResourceType: mapping.ResourceType,
		ResourceID:   resourceID,
		Region:       region,
		Name:         name,
		Tags:         tags,
		RawMetadata:  metadata,
	}
}

// tableMappings returns the table-to-resource-type mappings for the configured provider.
func (d *SteampipeDiscoverer) tableMappings() []tableMapping {
	if d.providerType == "oci" {
		return ociTableMappings()
	}
	return awsTableMappings()
}

func awsTableMappings() []tableMapping {
	return []tableMapping{
		{Table: "aws_vpc", ResourceType: provider.ResourceTypeVPC, IDColumn: "arn", NameColumn: "title", RegionColumn: "region"},
		{Table: "aws_vpc_subnet", ResourceType: provider.ResourceTypeSubnet, IDColumn: "subnet_arn", NameColumn: "title", RegionColumn: "region"},
		{Table: "aws_vpc_route_table", ResourceType: provider.ResourceTypeRouteTable, IDColumn: "route_table_id", NameColumn: "title", RegionColumn: "region"},
		{Table: "aws_vpc_internet_gateway", ResourceType: provider.ResourceTypeIGW, IDColumn: "internet_gateway_id", NameColumn: "title", RegionColumn: "region"},
		{Table: "aws_vpc_nat_gateway", ResourceType: provider.ResourceTypeNATGW, IDColumn: "arn", NameColumn: "title", RegionColumn: "region"},
		{Table: "aws_ec2_transit_gateway", ResourceType: provider.ResourceTypeTGW, IDColumn: "transit_gateway_arn", NameColumn: "title", RegionColumn: "region"},
		{Table: "aws_vpc_security_group", ResourceType: provider.ResourceTypeSecurityGroup, IDColumn: "arn", NameColumn: "group_name", RegionColumn: "region"},
		{Table: "aws_ec2_instance", ResourceType: provider.ResourceTypeEC2Instance, IDColumn: "arn", NameColumn: "title", RegionColumn: "region", FallbackColumns: []string{"arn", "instance_id", "instance_type", "instance_state", "region", "title", "tags", "vpc_id", "subnet_id", "private_ip_address", "public_ip_address"}},
		{Table: "aws_eks_cluster", ResourceType: provider.ResourceTypeEKSCluster, IDColumn: "arn", NameColumn: "name", RegionColumn: "region"},
		{Table: "aws_ecs_cluster", ResourceType: provider.ResourceTypeECSCluster, IDColumn: "cluster_arn", NameColumn: "cluster_name", RegionColumn: "region"},
		{Table: "aws_lambda_function", ResourceType: provider.ResourceTypeLambda, IDColumn: "arn", NameColumn: "name", RegionColumn: "region", FallbackColumns: []string{"arn", "name", "runtime", "memory_size", "region"}},
		{Table: "aws_rds_db_instance", ResourceType: provider.ResourceTypeRDS, IDColumn: "arn", NameColumn: "db_instance_identifier", RegionColumn: "region"},
		{Table: "aws_s3_bucket", ResourceType: provider.ResourceTypeS3Bucket, IDColumn: "arn", NameColumn: "name", RegionColumn: "region", FallbackColumns: []string{"arn", "name", "region", "creation_date", "tags"}},
		{Table: "aws_iam_user", ResourceType: provider.ResourceTypeIAMUser, IDColumn: "arn", NameColumn: "name", RegionColumn: "", FallbackColumns: []string{"arn", "name", "user_id", "create_date", "tags"}},
		{Table: "aws_iam_role", ResourceType: provider.ResourceTypeIAMRole, IDColumn: "arn", NameColumn: "name", RegionColumn: "", FallbackColumns: []string{"arn", "name", "role_id", "create_date", "tags"}},
		{Table: "aws_iam_policy", ResourceType: provider.ResourceTypeIAMPolicy, IDColumn: "arn", NameColumn: "name", RegionColumn: "", FallbackColumns: []string{"arn", "name", "policy_id", "is_aws_managed"}},
		{Table: "aws_ec2_application_load_balancer", ResourceType: provider.ResourceTypeALB, IDColumn: "arn", NameColumn: "name", RegionColumn: "region", FallbackColumns: []string{"arn", "name", "dns_name", "scheme", "type", "state_code", "region", "vpc_id"}},
		{Table: "aws_ec2_network_load_balancer", ResourceType: provider.ResourceTypeNLB, IDColumn: "arn", NameColumn: "name", RegionColumn: "region", FallbackColumns: []string{"arn", "name", "dns_name", "scheme", "type", "state_code", "region", "vpc_id"}},
		{Table: "aws_route53_zone", ResourceType: provider.ResourceTypeRoute53Zone, IDColumn: "id", NameColumn: "name", RegionColumn: ""},
		{Table: "aws_kms_key", ResourceType: provider.ResourceTypeKMSKey, IDColumn: "arn", NameColumn: "title", RegionColumn: "region", FallbackColumns: []string{"arn", "id", "title", "key_manager", "key_state", "region"}},
		{Table: "aws_secretsmanager_secret", ResourceType: provider.ResourceTypeSecret, IDColumn: "arn", NameColumn: "name", RegionColumn: "region", FallbackColumns: []string{"arn", "name", "region", "tags"}},
		{Table: "aws_ebs_volume", ResourceType: provider.ResourceTypeEBSVolume, IDColumn: "arn", NameColumn: "title", RegionColumn: "region"},
		{Table: "aws_efs_file_system", ResourceType: provider.ResourceTypeEFS, IDColumn: "arn", NameColumn: "name", RegionColumn: "region", FallbackColumns: []string{"arn", "file_system_id", "name", "life_cycle_state", "size_in_bytes", "region", "tags"}},
		{Table: "aws_eks_node_group", ResourceType: provider.ResourceTypeEKSNodeGroup, IDColumn: "arn", NameColumn: "nodegroup_name", RegionColumn: "region"},
		{Table: "aws_ec2_target_group", ResourceType: provider.ResourceTypeTargetGroup, IDColumn: "target_group_arn", NameColumn: "target_group_name", RegionColumn: "region", FallbackColumns: []string{"target_group_arn", "target_group_name", "protocol", "port", "vpc_id", "region"}},
	}
}

func ociTableMappings() []tableMapping {
	return []tableMapping{
		{Table: "oci_core_vcn", ResourceType: provider.ResourceTypeVCN, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_subnet", ResourceType: provider.ResourceTypeOCISubnet, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_route_table", ResourceType: provider.ResourceTypeOCIRouteTable, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_security_list", ResourceType: provider.ResourceTypeSecurityList, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_network_security_group", ResourceType: provider.ResourceTypeNSG, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_drg", ResourceType: provider.ResourceTypeDRG, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_internet_gateway", ResourceType: provider.ResourceTypeOCIIGW, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_nat_gateway", ResourceType: provider.ResourceTypeOCINATGW, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_service_gateway", ResourceType: provider.ResourceTypeServiceGateway, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_instance", ResourceType: provider.ResourceTypeComputeInstance, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_containerengine_cluster", ResourceType: provider.ResourceTypeOKECluster, IDColumn: "id", NameColumn: "name", RegionColumn: "region"},
		{Table: "oci_core_boot_volume", ResourceType: provider.ResourceTypeBlockVolume, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_volume", ResourceType: provider.ResourceTypeBlockVolume, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_objectstorage_bucket", ResourceType: provider.ResourceTypeObjectStorage, IDColumn: "name", NameColumn: "name", RegionColumn: "region"},
		{Table: "oci_identity_compartment", ResourceType: provider.ResourceTypeCompartment, IDColumn: "id", NameColumn: "name", RegionColumn: ""},
		{Table: "oci_database_db_system", ResourceType: provider.ResourceTypeOCIDB, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
		{Table: "oci_core_load_balancer", ResourceType: provider.ResourceTypeOCILB, IDColumn: "id", NameColumn: "display_name", RegionColumn: "region"},
	}
}

// getStringFromMap extracts a string value from the metadata map.
func getStringFromMap(m map[string]any, key string) string {
	if key == "" {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case *string:
		if val != nil {
			return *val
		}
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// extractTags attempts to extract tags from the metadata map.
// Steampipe typically stores tags as a jsonb column named "tags".
func extractTags(metadata map[string]any) map[string]string {
	tags := make(map[string]string)

	rawTags, ok := metadata["tags"]
	if !ok || rawTags == nil {
		return tags
	}

	switch v := rawTags.(type) {
	case map[string]any:
		for k, val := range v {
			if s, ok := val.(string); ok {
				tags[k] = s
			} else if val != nil {
				tags[k] = fmt.Sprintf("%v", val)
			}
		}
	case map[string]string:
		return v
	case string:
		// Try to parse as JSON.
		var parsed map[string]string
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return parsed
		}
	}

	return tags
}

// contains is a simple string-contains check for error classification.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
