package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chxmxii/3a/internal/storage"
)

type costView struct {
	costs        []storage.CostEstimate
	resources    []storage.Resource
	scrollOffset int
}

func (v *costView) render(width, height int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("  💰 Cost Analysis"))
	b.WriteString("\n\n")

	if len(v.costs) == 0 {
		b.WriteString(normalStyle.Render("  No cost estimates available."))
		return b.String()
	}

	lines := v.buildLines()

	maxRows := height - 6
	if maxRows < 10 {
		maxRows = 10
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
		b.WriteString(dimNavStyle.Render(fmt.Sprintf("\n  ↕ scroll %d%%", pct)))
	}

	return b.String()
}

func (v *costView) buildLines() []string {
	var lines []string

	totalCost := 0.0
	byCategory := make(map[string]float64)
	var idleResources []storage.CostEstimate
	var oversizedResources []storage.CostEstimate

	for _, c := range v.costs {
		if c.MonthlyCost != nil {
			totalCost += *c.MonthlyCost
			byCategory[c.Category] += *c.MonthlyCost
		}
		if c.IdleFlag {
			idleResources = append(idleResources, c)
		}
		if c.OversizedFlag {
			oversizedResources = append(oversizedResources, c)
		}
	}

	// Total.
	lines = append(lines, headerStyle.Render("  ┌─ Monthly Total"))
	lines = append(lines, fmt.Sprintf("  │ %s %s", keyStyle.Render("Monthly:"), passStyle.Render(fmt.Sprintf("$%.2f", totalCost))))
	lines = append(lines, fmt.Sprintf("  │ %s %s", keyStyle.Render("Annual: "), dimNavStyle.Render(fmt.Sprintf("~$%.2f", totalCost*12))))
	lines = append(lines, "  └─")
	lines = append(lines, "")

	// By category.
	lines = append(lines, headerStyle.Render("  ┌─ Breakdown by Category"))
	lines = append(lines, fmt.Sprintf("  │ %s  %s  %s  %s", keyStyle.Render("CATEGORY   "), keyStyle.Render("COST       "), keyStyle.Render(" %  "), keyStyle.Render("BAR")))
	lines = append(lines, "  │ "+strings.Repeat("─", 55))

	type catCost struct {
		name string
		cost float64
	}
	var cats []catCost
	for k, c := range byCategory {
		cats = append(cats, catCost{k, c})
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].cost > cats[j].cost })
	for _, cat := range cats {
		pct := 0.0
		if totalCost > 0 {
			pct = (cat.cost / totalCost) * 100
		}
		barLen := int(pct / 3)
		if barLen < 1 && cat.cost > 0 {
			barLen = 1
		}
		bar := passStyle.Render(strings.Repeat("█", barLen))
		costStr := fmt.Sprintf("$%.2f", cat.cost)
		lines = append(lines, fmt.Sprintf("  │ %-12s %s  %s  %s", cat.name, valueStyle.Render(fmt.Sprintf("%10s", costStr)), dimNavStyle.Render(fmt.Sprintf("%5.1f%%", pct)), bar))
	}
	lines = append(lines, "  └─")
	lines = append(lines, "")

	// Top cost drivers.
	lines = append(lines, headerStyle.Render("  ┌─ Top Cost Drivers"))
	nameMap := make(map[string]string)
	for _, r := range v.resources {
		nameMap[r.ResourceID] = r.Name
	}

	type costItem struct {
		id      string
		cost    float64
		resType string
	}
	var items []costItem
	for _, c := range v.costs {
		if c.MonthlyCost != nil && *c.MonthlyCost > 0 {
			items = append(items, costItem{c.ResourceID, *c.MonthlyCost, c.ResourceType})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].cost > items[j].cost })
	if len(items) > 10 {
		items = items[:10]
	}
	for i, item := range items {
		name := nameMap[item.id]
		if name == "" {
			name = item.id
		}
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		costColor := normalStyle
		if item.cost > 100 {
			costColor = severityHighStyle
		} else if item.cost > 50 {
			costColor = severityMediumStyle
		}
		lines = append(lines, fmt.Sprintf("  │ %2d. %-30s %s  %s", i+1, name, dimNavStyle.Render(fmt.Sprintf("%-14s", item.resType)), costColor.Render(fmt.Sprintf("$%.2f/mo", item.cost))))
	}
	lines = append(lines, "  └─")
	lines = append(lines, "")

	// Optimization.
	if len(idleResources) > 0 || len(oversizedResources) > 0 {
		lines = append(lines, headerStyle.Render("  ┌─ Optimization Opportunities"))
		if len(idleResources) > 0 {
			lines = append(lines, warnStyle.Render(fmt.Sprintf("  │ ⚠ %d potentially idle resource(s)", len(idleResources))))
		}
		if len(oversizedResources) > 0 {
			lines = append(lines, warnStyle.Render(fmt.Sprintf("  │ ⚠ %d potentially oversized resource(s)", len(oversizedResources))))
		}
		lines = append(lines, "  └─")
	}

	return lines
}
