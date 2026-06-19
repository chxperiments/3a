package tui

import (
	"fmt"
	"strings"

	"github.com/chxmxii/3a/internal/storage"
)

const (
	ArchModeNetwork  = "network"
	ArchModeResource = "resource"
)

type architectureView struct {
	resources     []storage.Resource
	relationships []storage.Relationship
	scrollOffset  int
	mode          string
}

func (v *architectureView) render(width, height int) string {
	if v.mode == "" {
		v.mode = ArchModeNetwork
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("  🏗️  Architecture"))
	b.WriteString("\n")

	// Mode indicator.
	networkLabel := "Network Architecture (n)"
	resourceLabel := "Resource Architecture (v)"
	if v.mode == ArchModeNetwork {
		networkLabel = selectedStyle.Render(" " + networkLabel + " ")
		resourceLabel = dimNavStyle.Render(" " + resourceLabel + " ")
	} else {
		networkLabel = dimNavStyle.Render(" " + networkLabel + " ")
		resourceLabel = selectedStyle.Render(" " + resourceLabel + " ")
	}
	b.WriteString("  " + networkLabel + dimNavStyle.Render(" | ") + resourceLabel)
	b.WriteString("\n\n")

	if len(v.resources) == 0 && len(v.relationships) == 0 {
		b.WriteString(normalStyle.Render("  No resources or relationships discovered."))
		b.WriteString("\n")
		b.WriteString(dimNavStyle.Render("  This can happen when resources lack cross-references or permissions are limited."))
		return b.String()
	}

	b.WriteString(dimNavStyle.Render(fmt.Sprintf("  %d resources, %d relationships mapped", len(v.resources), len(v.relationships))))
	b.WriteString("\n\n")

	// Build name/type lookups.
	nameMap := make(map[string]string)
	typeMap := make(map[string]string)
	for _, r := range v.resources {
		display := r.Name
		if display == "" {
			display = r.ResourceID
		}
		nameMap[r.ResourceID] = display
		typeMap[r.ResourceID] = r.ResourceType
	}

	var lines []string
	switch v.mode {
	case ArchModeNetwork:
		lines = v.buildNetworkLines(nameMap, typeMap)
	case ArchModeResource:
		lines = v.buildResourceLines(nameMap, typeMap)
	default:
		lines = v.buildNetworkLines(nameMap, typeMap)
	}

	if len(lines) == 0 {
		b.WriteString(dimNavStyle.Render("  No resources found for this view mode."))
		return b.String()
	}

	// Apply scroll.
	maxRows := height - 10
	if maxRows < 5 {
		maxRows = 5
	}
	if v.scrollOffset > len(lines)-maxRows {
		v.scrollOffset = max(0, len(lines)-maxRows)
	}
	end := v.scrollOffset + maxRows
	if end > len(lines) {
		end = len(lines)
	}

	for i := v.scrollOffset; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}

	if len(lines) > maxRows {
		pct := 0
		if len(lines)-maxRows > 0 {
			pct = (v.scrollOffset * 100) / (len(lines) - maxRows)
		}
		b.WriteString(dimNavStyle.Render(fmt.Sprintf("\n  ↕ scroll %d%% (%d lines)", pct, len(lines))))
	}

	return b.String()
}

// networkTypes are the resource types shown in the network view.
var networkTypes = map[string]bool{
	"vpc":              true,
	"subnet":           true,
	"route_table":      true,
	"security_group":   true,
	"internet_gateway": true,
	"nat_gateway":      true,
	"transit_gateway":  true,
	"ec2_instance":     true,
}

// resourceViewTypes are the resource types shown in the resource view.
var resourceViewTypes = map[string]bool{
	"alb":             true,
	"nlb":             true,
	"target_group":    true,
	"ec2_instance":    true,
	"eks_cluster":     true,
	"eks_node_group":  true,
	"ecs_cluster":     true,
	"lambda_function": true,
	"rds_instance":    true,
	"efs_file_system": true,
}

// buildNetworkLines builds the network topology view:
// VPC → Subnets → (Route Tables, EC2), Security Groups, Gateways
func (v *architectureView) buildNetworkLines(nameMap, typeMap map[string]string) []string {
	var lines []string

	// Index resources by type and ID.
	resourceByID := make(map[string]storage.Resource)
	resourcesByType := make(map[string][]storage.Resource)
	for _, r := range v.resources {
		if networkTypes[r.ResourceType] {
			resourceByID[r.ResourceID] = r
			resourcesByType[r.ResourceType] = append(resourcesByType[r.ResourceType], r)
		}
	}

	// Build relationship maps.
	// childrenOf[parentID] = list of child resource IDs
	childrenOf := make(map[string][]string)
	parentOf := make(map[string]string)
	for _, rel := range v.relationships {
		srcType := typeMap[rel.SourceID]
		tgtType := typeMap[rel.TargetID]
		if !networkTypes[srcType] || !networkTypes[tgtType] {
			continue
		}
		childrenOf[rel.SourceID] = append(childrenOf[rel.SourceID], rel.TargetID)
		parentOf[rel.TargetID] = rel.SourceID
	}

	// Also use metadata to associate subnets to VPCs if no explicit relationship.
	vpcForSubnet := make(map[string]string)
	for _, sub := range resourcesByType["subnet"] {
		// Check if already has parent via relationships.
		if _, hasParent := parentOf[sub.ResourceID]; hasParent {
			vpcForSubnet[sub.ResourceID] = parentOf[sub.ResourceID]
			continue
		}
		// Try metadata vpc_id.
		if sub.RawMetadata != nil {
			if vpcID, ok := sub.RawMetadata["vpc_id"].(string); ok && vpcID != "" {
				vpcForSubnet[sub.ResourceID] = vpcID
			}
		}
	}

	// Associate EC2 to subnets via metadata.
	subnetForEC2 := make(map[string]string)
	for _, ec2 := range resourcesByType["ec2_instance"] {
		if _, hasParent := parentOf[ec2.ResourceID]; hasParent {
			subnetForEC2[ec2.ResourceID] = parentOf[ec2.ResourceID]
			continue
		}
		if ec2.RawMetadata != nil {
			if subID, ok := ec2.RawMetadata["subnet_id"].(string); ok && subID != "" {
				subnetForEC2[ec2.ResourceID] = subID
			}
		}
	}

	// Associate SGs to VPCs via metadata.
	vpcForSG := make(map[string]string)
	for _, sg := range resourcesByType["security_group"] {
		if _, hasParent := parentOf[sg.ResourceID]; hasParent {
			vpcForSG[sg.ResourceID] = parentOf[sg.ResourceID]
			continue
		}
		if sg.RawMetadata != nil {
			if vpcID, ok := sg.RawMetadata["vpc_id"].(string); ok && vpcID != "" {
				vpcForSG[sg.ResourceID] = vpcID
			}
		}
	}

	// Associate Gateways to VPCs via metadata or relationships.
	vpcForGW := make(map[string]string)
	gwTypes := []string{"internet_gateway", "nat_gateway", "transit_gateway"}
	for _, gwType := range gwTypes {
		for _, gw := range resourcesByType[gwType] {
			if _, hasParent := parentOf[gw.ResourceID]; hasParent {
				vpcForGW[gw.ResourceID] = parentOf[gw.ResourceID]
				continue
			}
			if gw.RawMetadata != nil {
				if vpcID, ok := gw.RawMetadata["vpc_id"].(string); ok && vpcID != "" {
					vpcForGW[gw.ResourceID] = vpcID
				}
			}
		}
	}

	// Associate route tables to subnets via metadata or relationships.
	subnetForRT := make(map[string]string)
	for _, rt := range resourcesByType["route_table"] {
		if _, hasParent := parentOf[rt.ResourceID]; hasParent {
			subnetForRT[rt.ResourceID] = parentOf[rt.ResourceID]
			continue
		}
		if rt.RawMetadata != nil {
			if subID, ok := rt.RawMetadata["subnet_id"].(string); ok && subID != "" {
				subnetForRT[rt.ResourceID] = subID
			}
		}
	}

	// Render VPC trees.
	vpcs := resourcesByType["vpc"]
	if len(vpcs) == 0 {
		lines = append(lines, dimNavStyle.Render("  No VPCs discovered"))
		return lines
	}

	for vi, vpc := range vpcs {
		vpcName := nameMap[vpc.ResourceID]
		if vpcName == "" {
			vpcName = vpc.ResourceID
		}
		isLastVPC := vi == len(vpcs)-1
		vpcPrefix := "  ├── "
		vpcChildPrefix := "  │   "
		if isLastVPC {
			vpcPrefix = "  └── "
			vpcChildPrefix = "      "
		}

		lines = append(lines, fmt.Sprintf("%s%s %s", vpcPrefix, titleStyle.Render("[vpc]"), vpcName))

		// Collect children for this VPC.
		var vpcSubnets []storage.Resource
		for _, sub := range resourcesByType["subnet"] {
			if vpcForSubnet[sub.ResourceID] == vpc.ResourceID {
				vpcSubnets = append(vpcSubnets, sub)
			}
		}

		var vpcSGs []storage.Resource
		for _, sg := range resourcesByType["security_group"] {
			if vpcForSG[sg.ResourceID] == vpc.ResourceID {
				vpcSGs = append(vpcSGs, sg)
			}
		}

		var vpcGWs []storage.Resource
		for _, gwType := range gwTypes {
			for _, gw := range resourcesByType[gwType] {
				if vpcForGW[gw.ResourceID] == vpc.ResourceID {
					vpcGWs = append(vpcGWs, gw)
				}
			}
		}

		totalVPCChildren := len(vpcSubnets) + len(vpcSGs) + len(vpcGWs)
		childIdx := 0

		// Subnets.
		for _, sub := range vpcSubnets {
			childIdx++
			isLastChild := childIdx == totalVPCChildren
			subConnector := "├── "
			subChildPrefix := vpcChildPrefix + "│   "
			if isLastChild {
				subConnector = "└── "
				subChildPrefix = vpcChildPrefix + "    "
			}

			subName := nameMap[sub.ResourceID]
			if subName == "" {
				subName = sub.ResourceID
			}
			lines = append(lines, fmt.Sprintf("%s%s%s %s", vpcChildPrefix, subConnector, normalStyle.Render("[subnet]"), subName))

			// Route tables and EC2 in this subnet.
			var subChildren []storage.Resource
			for _, rt := range resourcesByType["route_table"] {
				if subnetForRT[rt.ResourceID] == sub.ResourceID {
					subChildren = append(subChildren, rt)
				}
			}
			for _, ec2 := range resourcesByType["ec2_instance"] {
				if subnetForEC2[ec2.ResourceID] == sub.ResourceID {
					subChildren = append(subChildren, ec2)
				}
			}

			for sci, sc := range subChildren {
				isLastSC := sci == len(subChildren)-1
				scConnector := "├── "
				if isLastSC {
					scConnector = "└── "
				}
				scName := nameMap[sc.ResourceID]
				if scName == "" {
					scName = sc.ResourceID
				}
				lines = append(lines, fmt.Sprintf("%s%s%s %s", subChildPrefix, scConnector, dimNavStyle.Render("["+sc.ResourceType+"]"), scName))
			}
		}

		// Security Groups.
		for _, sg := range vpcSGs {
			childIdx++
			isLastChild := childIdx == totalVPCChildren
			sgConnector := "├── "
			if isLastChild {
				sgConnector = "└── "
			}
			sgName := nameMap[sg.ResourceID]
			if sgName == "" {
				sgName = sg.ResourceID
			}
			lines = append(lines, fmt.Sprintf("%s%s%s %s", vpcChildPrefix, sgConnector, dimNavStyle.Render("[security_group]"), sgName))
		}

		// Gateways.
		for _, gw := range vpcGWs {
			childIdx++
			isLastChild := childIdx == totalVPCChildren
			gwConnector := "├── "
			if isLastChild {
				gwConnector = "└── "
			}
			gwName := nameMap[gw.ResourceID]
			if gwName == "" {
				gwName = gw.ResourceID
			}
			lines = append(lines, fmt.Sprintf("%s%s%s %s", vpcChildPrefix, gwConnector, dimNavStyle.Render("["+gw.ResourceType+"]"), gwName))
		}
	}

	return lines
}

// buildResourceLines builds the application-level resource view.
func (v *architectureView) buildResourceLines(nameMap, typeMap map[string]string) []string {
	var lines []string

	// Index resources by type.
	resourcesByType := make(map[string][]storage.Resource)
	for _, r := range v.resources {
		if resourceViewTypes[r.ResourceType] {
			resourcesByType[r.ResourceType] = append(resourcesByType[r.ResourceType], r)
		}
	}

	// Build relationship map (source → targets) for resource view types only.
	childrenOf := make(map[string][]string)
	parentOf := make(map[string]string)
	for _, rel := range v.relationships {
		srcType := typeMap[rel.SourceID]
		tgtType := typeMap[rel.TargetID]
		if !resourceViewTypes[srcType] || !resourceViewTypes[tgtType] {
			continue
		}
		childrenOf[rel.SourceID] = append(childrenOf[rel.SourceID], rel.TargetID)
		parentOf[rel.TargetID] = rel.SourceID
	}

	// Root resources are those that have no parent within the resource view types.
	var roots []storage.Resource
	for _, r := range v.resources {
		if !resourceViewTypes[r.ResourceType] {
			continue
		}
		if _, hasParent := parentOf[r.ResourceID]; !hasParent {
			roots = append(roots, r)
		}
	}

	if len(roots) == 0 {
		lines = append(lines, dimNavStyle.Render("  No application resources discovered"))
		return lines
	}

	rendered := make(map[string]bool)
	for ri, root := range roots {
		if rendered[root.ResourceID] {
			continue
		}
		isLast := ri == len(roots)-1
		v.buildResourceTree(&lines, root.ResourceID, "  ", isLast, childrenOf, nameMap, typeMap, rendered, 0)
	}

	return lines
}

func (v *architectureView) buildResourceTree(lines *[]string, resourceID, prefix string, isLast bool, childrenOf map[string][]string, nameMap, typeMap map[string]string, rendered map[string]bool, depth int) {
	if depth > 5 || rendered[resourceID] {
		return
	}
	rendered[resourceID] = true

	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if depth == 0 {
		connector = ""
	}

	name := nameMap[resourceID]
	if name == "" {
		name = resourceID
	}
	if len(name) > 40 {
		name = name[:37] + "..."
	}
	rType := typeMap[resourceID]
	if rType == "" {
		rType = "?"
	}

	line := fmt.Sprintf("%s%s%s %s", prefix, connector, normalStyle.Render("["+rType+"]"), name)
	*lines = append(*lines, line)

	children := childrenOf[resourceID]
	childPrefix := prefix + "│   "
	if isLast || depth == 0 {
		childPrefix = prefix + "    "
	}

	for i, childID := range children {
		isChildLast := i == len(children)-1
		v.buildResourceTree(lines, childID, childPrefix, isChildLast, childrenOf, nameMap, typeMap, rendered, depth+1)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
