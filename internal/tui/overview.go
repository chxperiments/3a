package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chxmxii/3a/internal/storage"
)

type overviewView struct {
	assessment   *storage.Assessment
	resources    []storage.Resource
	findings     []storage.Finding
	costs        []storage.CostEstimate
	scrollOffset int
}

func (v *overviewView) render(width, height int) string {
	lines := v.buildLines()

	// Apply scroll.
	if v.scrollOffset > len(lines)-height {
		v.scrollOffset = max(0, len(lines)-height)
	}
	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}

	end := v.scrollOffset + height
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := v.scrollOffset; i < end; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}

	if len(lines) > height {
		pct := 0
		if len(lines)-height > 0 {
			pct = (v.scrollOffset * 100) / (len(lines) - height)
		}
		b.WriteString(dimNavStyle.Render(fmt.Sprintf("  ↕ %d%%", pct)))
	}

	return b.String()
}

func (v *overviewView) buildLines() []string {
	var lines []string

	lines = append(lines, titleStyle.Render("  📊 Assessment Overview"))
	lines = append(lines, "")

	if v.assessment != nil {
		lines = append(lines, headerStyle.Render("  ┌─ Assessment Info"))
		lines = append(lines, fmt.Sprintf("  │ Profile:   %s", v.assessment.Profile))
		lines = append(lines, fmt.Sprintf("  │ Provider:  %s", v.assessment.Provider))
		lines = append(lines, fmt.Sprintf("  │ Status:    %s", v.assessment.Status))
		lines = append(lines, fmt.Sprintf("  │ Started:   %s", v.assessment.StartedAt.Format("2006-01-02 15:04:05")))
		if v.assessment.CompletedAt != nil {
			lines = append(lines, fmt.Sprintf("  │ Completed: %s", v.assessment.CompletedAt.Format("2006-01-02 15:04:05")))
		}
		lines = append(lines, fmt.Sprintf("  │ Regions:   %s", strings.Join(v.assessment.Regions, ", ")))
		lines = append(lines, "  └─")
		lines = append(lines, "")
	}

	// Resources summary.
	lines = append(lines, headerStyle.Render(fmt.Sprintf("  ┌─ Resources (%d)", len(v.resources))))
	typeCounts := make(map[string]int)
	regionCounts := make(map[string]int)
	for _, r := range v.resources {
		typeCounts[r.ResourceType]++
		regionCounts[r.Region]++
	}

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range typeCounts {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	for _, item := range sorted {
		lines = append(lines, fmt.Sprintf("  │ %-22s %d", item.key, item.count))
	}
	lines = append(lines, "  └─")
	lines = append(lines, "")

	// Regions.
	lines = append(lines, headerStyle.Render(fmt.Sprintf("  ┌─ Regions (%d)", len(regionCounts))))
	var regionSorted []kv
	for k, c := range regionCounts {
		regionSorted = append(regionSorted, kv{k, c})
	}
	sort.Slice(regionSorted, func(i, j int) bool { return regionSorted[i].count > regionSorted[j].count })
	for _, item := range regionSorted {
		lines = append(lines, fmt.Sprintf("  │ %-22s %d resources", item.key, item.count))
	}
	lines = append(lines, "  └─")
	lines = append(lines, "")

	// Findings.
	lines = append(lines, headerStyle.Render(fmt.Sprintf("  ┌─ Findings (%d)", len(v.findings))))
	if len(v.findings) == 0 {
		lines = append(lines, passStyle.Render("  │ ✓ No findings — all checks passed"))
	} else {
		sevCounts := map[string]int{}
		for _, f := range v.findings {
			sevCounts[f.Severity]++
		}
		if c := sevCounts["critical"]; c > 0 {
			lines = append(lines, fmt.Sprintf("  │ %s  %d", severityCriticalStyle.Render("● CRITICAL"), c))
		}
		if c := sevCounts["high"]; c > 0 {
			lines = append(lines, fmt.Sprintf("  │ %s      %d", severityHighStyle.Render("● HIGH"), c))
		}
		if c := sevCounts["medium"]; c > 0 {
			lines = append(lines, fmt.Sprintf("  │ %s    %d", severityMediumStyle.Render("● MEDIUM"), c))
		}
		if c := sevCounts["low"]; c > 0 {
			lines = append(lines, fmt.Sprintf("  │ %s       %d", severityLowStyle.Render("● LOW"), c))
		}
	}
	lines = append(lines, "  └─")
	lines = append(lines, "")

	// Cost.
	totalCost := 0.0
	for _, c := range v.costs {
		if c.MonthlyCost != nil {
			totalCost += *c.MonthlyCost
		}
	}
	lines = append(lines, headerStyle.Render("  ┌─ Monthly Cost"))
	lines = append(lines, fmt.Sprintf("  │ $%.2f/month  (~$%.2f/year)", totalCost, totalCost*12))
	lines = append(lines, "  └─")

	return lines
}
