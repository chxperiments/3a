package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chxmxii/3a/internal/storage"
)

// View represents the currently active TUI view.
type View int

const (
	ViewOverview View = iota
	ViewInventory
	ViewArchitecture
	ViewFindings
	ViewCost
)

// Model is the root Bubble Tea model for the 3A TUI.
type Model struct {
	store        *storage.Store
	assessmentID string
	activeView   View
	width        int
	height       int

	// View models.
	overview     overviewView
	inventory    inventoryView
	architecture architectureView
	findings     findingsView
	cost         costView

	loaded bool
	err    error
}

// dataLoadedMsg is sent when data has been loaded from the store.
type dataLoadedMsg struct {
	overview     overviewView
	inventory    inventoryView
	architecture architectureView
	findings     findingsView
	cost         costView
}

// errMsg wraps an error for the TUI.
type errMsg struct{ err error }

// NewModel creates a new TUI model for the given assessment.
func NewModel(store *storage.Store, assessmentID string) Model {
	return Model{
		store:        store,
		assessmentID: assessmentID,
		activeView:   ViewOverview,
	}
}

// Init starts the TUI.
func (m Model) Init() tea.Cmd {
	return m.loadData
}

func (m Model) loadData() tea.Msg {
	assessment, err := m.store.GetAssessment(m.assessmentID)
	if err != nil {
		return errMsg{err}
	}

	resources, err := m.store.GetResourcesByAssessment(m.assessmentID)
	if err != nil {
		return errMsg{err}
	}

	findings, err := m.store.GetFindingsByAssessment(m.assessmentID)
	if err != nil {
		return errMsg{err}
	}

	relationships, err := m.store.GetRelationshipsByAssessment(m.assessmentID)
	if err != nil {
		return errMsg{err}
	}

	costs, err := m.store.GetCostsByAssessment(m.assessmentID)
	if err != nil {
		return errMsg{err}
	}

	// Build region list from resources.
	regionSet := make(map[string]bool)
	for _, r := range resources {
		if r.Region != "" {
			regionSet[r.Region] = true
		}
	}
	var regions []string
	for reg := range regionSet {
		regions = append(regions, reg)
	}
	sort.Strings(regions)

	return dataLoadedMsg{
		overview: overviewView{
			assessment: assessment,
			resources:  resources,
			findings:   findings,
			costs:      costs,
		},
		inventory: inventoryView{
			resources: resources,
			regions:   regions,
		},
		architecture: architectureView{
			resources:     resources,
			relationships: relationships,
		},
		findings: findingsView{findings: findings},
		cost:     costView{costs: costs, resources: resources},
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.activeView = ViewOverview
		case "2":
			m.activeView = ViewInventory
		case "3":
			m.activeView = ViewArchitecture
		case "4":
			m.activeView = ViewFindings
		case "5":
			m.activeView = ViewCost

		// Navigation.
		case "up", "k":
			m.handleUp()
		case "down", "j":
			m.handleDown()

		// Region cycling in Inventory.
		case "r":
			if m.activeView == ViewInventory {
				m.inventory.nextRegion()
			}
		case "R":
			if m.activeView == ViewInventory {
				m.inventory.prevRegion()
			}

		// Type filter in Inventory.
		case "t":
			if m.activeView == ViewInventory {
				m.inventory.nextType()
			}

		// Clear filters.
		case "x":
			if m.activeView == ViewInventory {
				m.inventory.clearFilters()
			}
			if m.activeView == ViewFindings {
				m.findings.severityFilter = ""
				m.findings.cursor = 0
			}

		// Findings severity filters.
		case "c":
			if m.activeView == ViewFindings {
				m.findings.severityFilter = "critical"
				m.findings.cursor = 0
			}
		case "h":
			if m.activeView == ViewFindings {
				m.findings.severityFilter = "high"
				m.findings.cursor = 0
			}
		case "m":
			if m.activeView == ViewFindings {
				m.findings.severityFilter = "medium"
				m.findings.cursor = 0
			}
		case "l":
			if m.activeView == ViewFindings {
				m.findings.severityFilter = "low"
				m.findings.cursor = 0
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case dataLoadedMsg:
		m.loaded = true
		m.overview = msg.overview
		m.inventory = msg.inventory
		m.architecture = msg.architecture
		m.findings = msg.findings
		m.cost = msg.cost

	case errMsg:
		m.err = msg.err
	}

	return m, nil
}

func (m *Model) handleUp() {
	switch m.activeView {
	case ViewOverview:
		if m.overview.scrollOffset > 0 {
			m.overview.scrollOffset--
		}
	case ViewInventory:
		if m.inventory.cursor > 0 {
			m.inventory.cursor--
			if m.inventory.cursor < m.inventory.offset {
				m.inventory.offset = m.inventory.cursor
			}
		}
	case ViewFindings:
		if m.findings.cursor > 0 {
			m.findings.cursor--
			if m.findings.cursor < m.findings.offset {
				m.findings.offset = m.findings.cursor
			}
		}
	case ViewArchitecture:
		if m.architecture.scrollOffset > 0 {
			m.architecture.scrollOffset--
		}
	case ViewCost:
		if m.cost.scrollOffset > 0 {
			m.cost.scrollOffset--
		}
	}
}

func (m *Model) handleDown() {
	switch m.activeView {
	case ViewOverview:
		m.overview.scrollOffset++
	case ViewInventory:
		filtered := m.inventory.filteredResources()
		if m.inventory.cursor < len(filtered)-1 {
			m.inventory.cursor++
			maxVisible := m.height - 12
			if maxVisible < 5 {
				maxVisible = 5
			}
			if m.inventory.cursor >= m.inventory.offset+maxVisible {
				m.inventory.offset++
			}
		}
	case ViewFindings:
		filtered := m.findings.filteredFindings()
		if m.findings.cursor < len(filtered)-1 {
			m.findings.cursor++
			maxVisible := m.height - 12
			if maxVisible < 5 {
				maxVisible = 5
			}
			if m.findings.cursor >= m.findings.offset+maxVisible {
				m.findings.offset++
			}
		}
	case ViewArchitecture:
		m.architecture.scrollOffset++
	case ViewCost:
		m.cost.scrollOffset++
	}
}

// View renders the TUI.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press q to quit.\n", m.err)
	}

	if !m.loaded {
		return "\n  Loading assessment data...\n"
	}

	// Fixed layout: nav (2 lines) + content (fills) + help (1 line).
	nav := m.renderNav()
	help := m.renderHelp()

	// Content area height = total height - nav (2) - help (2) - borders.
	contentHeight := m.height - 5
	if contentHeight < 10 {
		contentHeight = 10
	}

	var content string
	switch m.activeView {
	case ViewOverview:
		content = m.overview.render(m.width, contentHeight)
	case ViewInventory:
		content = m.inventory.render(m.width, contentHeight)
	case ViewArchitecture:
		content = m.architecture.render(m.width, contentHeight)
	case ViewFindings:
		content = m.findings.render(m.width, contentHeight)
	case ViewCost:
		content = m.cost.render(m.width, contentHeight)
	}

	// Truncate content if it exceeds available height.
	contentLines := strings.Split(content, "\n")
	if len(contentLines) > contentHeight {
		contentLines = contentLines[:contentHeight]
	}
	content = strings.Join(contentLines, "\n")

	return nav + "\n" + content + "\n\n" + help
}

func (m Model) renderNav() string {
	tabs := []struct {
		key  string
		name string
		view View
	}{
		{"1", "Overview", ViewOverview},
		{"2", "Inventory", ViewInventory},
		{"3", "Architecture", ViewArchitecture},
		{"4", "Findings", ViewFindings},
		{"5", "Cost", ViewCost},
	}

	var parts []string
	for _, tab := range tabs {
		label := fmt.Sprintf(" %s %s ", tab.key, tab.name)
		if tab.view == m.activeView {
			parts = append(parts, selectedStyle.Render(label))
		} else {
			parts = append(parts, dimNavStyle.Render(label))
		}
	}

	return "\n " + joinStrings(parts, dimNavStyle.Render("│"))
}

func (m Model) renderHelp() string {
	base := "q:quit  ↑↓:scroll  1-5:views"
	switch m.activeView {
	case ViewInventory:
		base += "  r/R:region  t:type  x:clear"
	case ViewFindings:
		base += "  c/h/m/l:severity  x:clear"
	}
	return helpStyle.Render("  " + base)
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
