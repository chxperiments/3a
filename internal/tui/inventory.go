package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chxmxii/3a/internal/storage"
)

type inventoryView struct {
	resources    []storage.Resource
	regions      []string
	regionIdx    int    // -1 means "all regions"
	typeFilter   string // empty means all types
	typeIdx      int
	cursor       int
	offset       int
	showDetail   bool // true when viewing a resource's details
	detailScroll int
}

func (v *inventoryView) nextRegion() {
	if v.showDetail {
		return
	}
	v.regionIdx++
	if v.regionIdx >= len(v.regions) {
		v.regionIdx = -1
	}
	v.cursor = 0
	v.offset = 0
}

func (v *inventoryView) prevRegion() {
	if v.showDetail {
		return
	}
	v.regionIdx--
	if v.regionIdx < -1 {
		v.regionIdx = len(v.regions) - 1
	}
	v.cursor = 0
	v.offset = 0
}

func (v *inventoryView) nextType() {
	if v.showDetail {
		return
	}
	types := v.availableTypes()
	v.typeIdx++
	if v.typeIdx >= len(types) {
		v.typeIdx = -1
		v.typeFilter = ""
	} else {
		v.typeFilter = types[v.typeIdx]
	}
	v.cursor = 0
	v.offset = 0
}

func (v *inventoryView) clearFilters() {
	if v.showDetail {
		v.showDetail = false
		v.detailScroll = 0
		return
	}
	v.regionIdx = -1
	v.typeFilter = ""
	v.typeIdx = -1
	v.cursor = 0
	v.offset = 0
}

func (v *inventoryView) toggleDetail() {
	if v.showDetail {
		v.showDetail = false
		v.detailScroll = 0
	} else {
		v.showDetail = true
		v.detailScroll = 0
	}
}

func (v *inventoryView) availableTypes() []string {
	typeSet := make(map[string]bool)
	for _, r := range v.resources {
		typeSet[r.ResourceType] = true
	}
	var types []string
	for t := range typeSet {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

func (v *inventoryView) currentRegion() string {
	if v.regionIdx < 0 || v.regionIdx >= len(v.regions) {
		return ""
	}
	return v.regions[v.regionIdx]
}

func (v *inventoryView) filteredResources() []storage.Resource {
	region := v.currentRegion()
	var filtered []storage.Resource
	for _, r := range v.resources {
		if region != "" && r.Region != region {
			continue
		}
		if v.typeFilter != "" && r.ResourceType != v.typeFilter {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func (v *inventoryView) selectedResource() *storage.Resource {
	filtered := v.filteredResources()
	if v.cursor < 0 || v.cursor >= len(filtered) {
		return nil
	}
	r := filtered[v.cursor]
	return &r
}

func (v *inventoryView) render(width, height int) string {
	if v.showDetail {
		return v.renderDetail(width, height)
	}
	return v.renderList(width, height)
}

func (v *inventoryView) renderList(width, height int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("  📦 Resource Inventory"))
	b.WriteString("\n\n")

	// Filter status bar.
	region := v.currentRegion()
	filterParts := []string{}
	if region != "" {
		filterParts = append(filterParts, regionBadgeStyle.Render("Region: "+region))
	} else {
		filterParts = append(filterParts, dimNavStyle.Render("Region: all"))
	}
	if v.typeFilter != "" {
		filterParts = append(filterParts, regionBadgeStyle.Render("Type: "+v.typeFilter))
	} else {
		filterParts = append(filterParts, dimNavStyle.Render("Type: all"))
	}
	b.WriteString("  " + strings.Join(filterParts, "  "))
	b.WriteString("\n")

	filtered := v.filteredResources()
	b.WriteString(dimNavStyle.Render(fmt.Sprintf("  %d of %d resources  (Enter: details)", len(filtered), len(v.resources))))
	b.WriteString("\n\n")

	// Table header.
	header := fmt.Sprintf("  %-18s %-35s %-18s %s", "TYPE", "NAME", "REGION", "ID")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(dimNavStyle.Render("  " + strings.Repeat("─", min(width-4, 95))))
	b.WriteString("\n")

	// Visible rows.
	maxRows := height - 11
	if maxRows < 5 {
		maxRows = 5
	}

	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+maxRows {
		v.offset = v.cursor - maxRows + 1
	}
	if v.offset < 0 {
		v.offset = 0
	}

	end := v.offset + maxRows
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := v.offset; i < end; i++ {
		r := filtered[i]
		shortID := r.ResourceID
		if len(shortID) > 20 {
			shortID = "..." + shortID[len(shortID)-17:]
		}
		name := r.Name
		if name == "" {
			name = "(unnamed)"
		}
		if len(name) > 33 {
			name = name[:30] + "..."
		}

		line := fmt.Sprintf("  %-18s %-35s %-18s %s", r.ResourceType, name, r.Region, shortID)
		if i == v.cursor {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	if len(filtered) > maxRows {
		pct := 0
		if len(filtered)-maxRows > 0 {
			pct = (v.offset * 100) / (len(filtered) - maxRows)
		}
		b.WriteString(dimNavStyle.Render(fmt.Sprintf("\n  ↕ scroll %d%%", pct)))
	}

	return b.String()
}

func (v *inventoryView) renderDetail(width, height int) string {
	r := v.selectedResource()
	if r == nil {
		v.showDetail = false
		return v.renderList(width, height)
	}

	lines := v.buildDetailLines(r, width)

	// Apply scroll.
	maxRows := height - 4
	if maxRows < 10 {
		maxRows = 10
	}
	if v.detailScroll > len(lines)-maxRows {
		v.detailScroll = max(0, len(lines)-maxRows)
	}
	if v.detailScroll < 0 {
		v.detailScroll = 0
	}

	end := v.detailScroll + maxRows
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := v.detailScroll; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}

	if len(lines) > maxRows {
		pct := 0
		if len(lines)-maxRows > 0 {
			pct = (v.detailScroll * 100) / (len(lines) - maxRows)
		}
		b.WriteString(dimNavStyle.Render(fmt.Sprintf("  ↕ %d%%  (Esc/x: back)", pct)))
	} else {
		b.WriteString(dimNavStyle.Render("  Esc/x: back to list"))
	}

	return b.String()
}

func (v *inventoryView) buildDetailLines(r *storage.Resource, width int) []string {
	var lines []string

	lines = append(lines, titleStyle.Render(fmt.Sprintf("  Resource: %s", r.Name)))
	lines = append(lines, "")

	// Core fields.
	lines = append(lines, headerStyle.Render("  ┌─ Identity"))
	lines = append(lines, fmt.Sprintf("  │ ID:     %s", r.ResourceID))
	lines = append(lines, fmt.Sprintf("  │ Type:   %s", r.ResourceType))
	lines = append(lines, fmt.Sprintf("  │ Name:   %s", r.Name))
	lines = append(lines, fmt.Sprintf("  │ Region: %s", r.Region))
	lines = append(lines, "  └─")
	lines = append(lines, "")

	// Tags.
	if len(r.Tags) > 0 {
		lines = append(lines, headerStyle.Render(fmt.Sprintf("  ┌─ Tags (%d)", len(r.Tags))))
		keys := make([]string, 0, len(r.Tags))
		for k := range r.Tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := r.Tags[k]
			if len(val) > 60 {
				val = val[:57] + "..."
			}
			lines = append(lines, fmt.Sprintf("  │ %-25s %s", k+":", val))
		}
		lines = append(lines, "  └─")
		lines = append(lines, "")
	}

	// Metadata.
	if len(r.RawMetadata) > 0 {
		lines = append(lines, headerStyle.Render(fmt.Sprintf("  ┌─ Metadata (%d fields)", len(r.RawMetadata))))

		keys := make([]string, 0, len(r.RawMetadata))
		for k := range r.RawMetadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			val := r.RawMetadata[k]
			valStr := formatMetadataValue(val, width-30)
			lines = append(lines, fmt.Sprintf("  │ %-25s %s", k+":", valStr))
		}
		lines = append(lines, "  └─")
	}

	return lines
}

func formatMetadataValue(val any, maxLen int) string {
	if val == nil {
		return dimNavStyle.Render("null")
	}
	if maxLen < 20 {
		maxLen = 20
	}

	switch v := val.(type) {
	case string:
		if len(v) > maxLen {
			return v[:maxLen-3] + "..."
		}
		return v
	case bool:
		if v {
			return passStyle.Render("true")
		}
		return failStyle.Render("false")
	case float64:
		if v == float64(int(v)) {
			return fmt.Sprintf("%d", int(v))
		}
		return fmt.Sprintf("%.2f", v)
	case map[string]any:
		if len(v) == 0 {
			return dimNavStyle.Render("{}")
		}
		parts := []string{}
		for k, inner := range v {
			s := fmt.Sprintf("%v", inner)
			if len(s) > 20 {
				s = s[:17] + "..."
			}
			parts = append(parts, k+"="+s)
			if len(parts) >= 3 {
				parts = append(parts, "...")
				break
			}
		}
		result := "{" + strings.Join(parts, ", ") + "}"
		if len(result) > maxLen {
			result = result[:maxLen-3] + "..."
		}
		return result
	case []any:
		if len(v) == 0 {
			return dimNavStyle.Render("[]")
		}
		return fmt.Sprintf("[%d items]", len(v))
	default:
		s := fmt.Sprintf("%v", v)
		if len(s) > maxLen {
			s = s[:maxLen-3] + "..."
		}
		return s
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
