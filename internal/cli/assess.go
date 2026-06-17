package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/chxmxii/3a/internal/architecture"
	"github.com/chxmxii/3a/internal/assessment"
	awsrules "github.com/chxmxii/3a/internal/assessment/rules/aws"
	ocirules "github.com/chxmxii/3a/internal/assessment/rules/oci"
	"github.com/chxmxii/3a/internal/checklist"
	"github.com/chxmxii/3a/internal/config"
	"github.com/chxmxii/3a/internal/cost"
	"github.com/chxmxii/3a/internal/discovery"
	"github.com/chxmxii/3a/internal/provider/steampipe"
	"github.com/chxmxii/3a/internal/sizing"
	"github.com/chxmxii/3a/internal/storage"
	"github.com/chxmxii/3a/internal/tui"
)

var (
	frames      = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true)
	doneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	failStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
)

// spinner runs an animated spinner while a task executes.
type spinner struct {
	mu      sync.Mutex
	msg     string
	running bool
	done    chan struct{}
}

func newSpinner(msg string) *spinner {
	s := &spinner{msg: msg, done: make(chan struct{})}
	s.start()
	return s
}

func (s *spinner) start() {
	s.running = true
	go func() {
		i := 0
		for {
			select {
			case <-s.done:
				return
			default:
				s.mu.Lock()
				frame := accentStyle.Render(frames[i%len(frames)])
				fmt.Printf("\r  %s %s", frame, s.msg)
				s.mu.Unlock()
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

func (s *spinner) succeed(result string) {
	close(s.done)
	s.running = false
	fmt.Printf("\r  %s %s\n", doneStyle.Render("✓"), result)
}

func (s *spinner) fail(result string) {
	close(s.done)
	s.running = false
	fmt.Printf("\r  %s %s\n", failStyle.Render("✗"), dimStyle.Render(result))
}

func newAssessCmd() *cobra.Command {
	var connString string
	var noTUI bool

	cmd := &cobra.Command{
		Use:   "assess <profile>",
		Short: "Run a full assessment for a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]
			return runAssessment(profileName, connString, noTUI)
		},
	}

	cmd.Flags().StringVar(&connString, "steampipe-conn", "postgres://steampipe@localhost:9193/steampipe", "Steampipe connection string")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "skip TUI and print summary to stdout")

	return cmd
}

func runAssessment(profileName, connString string, noTUI bool) error {
	ctx := context.Background()

	// Suppress steampipe log noise.
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	// Ensure ~/.3a directory exists.
	if _, err := config.EnsureConfigDir(); err != nil {
		return err
	}

	// Load config.
	cfgPath := config.DefaultConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = &config.Config{
			DBPath: resolveDBPath(getDBPath()),
			Profiles: []config.AccountProfile{
				{
					Name:     profileName,
					Provider: "aws",
					Regions:  []string{"us-east-1"},
				},
			},
		}
	}

	profile, err := config.GetProfile(cfg, profileName)
	if err != nil {
		return fmt.Errorf("profile error: %w", err)
	}

	// Open storage.
	dbFile := resolveDBPath(getDBPath())
	if cfg.DBPath != "" {
		dbFile = resolveDBPath(cfg.DBPath)
	}
	store, err := storage.Open(dbFile)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	// Create assessment.
	assessmentID := uuid.New().String()
	now := time.Now()
	a := &storage.Assessment{
		ID:        assessmentID,
		Profile:   profileName,
		Provider:  profile.Provider,
		Status:    "in_progress",
		StartedAt: now,
		Regions:   profile.Regions,
	}
	if err := store.CreateAssessment(a); err != nil {
		return fmt.Errorf("creating assessment: %w", err)
	}

	// Header.
	fmt.Println()
	fmt.Printf("  %s\n", accentStyle.Render("━━━ 3A — Agnostic Account Assessment ━━━"))
	fmt.Printf("  %s\n\n", dimStyle.Render(fmt.Sprintf("Profile: %s  Provider: %s  ID: %s", profileName, profile.Provider, assessmentID[:8])))

	// Step 1: Connect.
	sp1 := newSpinner("Connecting to Steampipe...")
	sp, err := steampipe.NewSteampipeProvider(connString, profile.Provider)
	if err != nil {
		sp1.fail("Connection failed")
		return fmt.Errorf("creating steampipe provider: %w", err)
	}
	defer sp.Close()
	if err := sp.Authenticate(ctx); err != nil {
		sp1.fail("Connection failed")
		return fmt.Errorf("connecting to steampipe: %w", err)
	}
	sp1.succeed("Connected to Steampipe")

	// Step 2: Validate.
	sp2 := newSpinner("Validating credentials...")
	if err := sp.ValidateProfile(ctx); err != nil {
		sp2.fail("Validation failed")
		_ = store.UpdateAssessmentStatus(assessmentID, "failed", nil)
		return fmt.Errorf("profile validation failed:\n\n%w", err)
	}
	sp2.succeed("Credentials validated")

	// Step 3: Discover.
	sp3 := newSpinner("Discovering resources (this may take a moment)...")
	engine := discovery.NewEngine(sp, store)
	summary, err := engine.Run(ctx, assessmentID, profile.Regions)
	if err != nil {
		sp3.fail("Discovery failed")
		return fmt.Errorf("discovery failed: %w", err)
	}
	if summary.TotalResources == 0 {
		sp3.fail("No resources found")
		_ = store.UpdateAssessmentStatus(assessmentID, "failed", nil)
		return fmt.Errorf("discovery returned 0 resources — check Steampipe credentials")
	}
	sp3.succeed(fmt.Sprintf("Discovered %d resources across %d regions", summary.TotalResources, len(summary.ByRegion)))

	// Step 4: Architecture.
	sp4 := newSpinner("Reconstructing architecture...")
	reconstructor := architecture.NewReconstructor(store, profile.Provider)
	if err := reconstructor.Reconstruct(assessmentID); err != nil {
		sp4.fail("Architecture: " + err.Error())
	} else {
		rels, _ := store.GetRelationshipsByAssessment(assessmentID)
		sp4.succeed(fmt.Sprintf("Mapped %d relationships", len(rels)))
	}

	// Step 5: Assessment.
	sp5 := newSpinner("Running security assessment...")
	var rules []assessment.Rule
	switch profile.Provider {
	case "aws":
		rules = awsrules.AllRules()
	case "oci":
		rules = ocirules.AllRules()
	}
	assessEngine := assessment.NewEngine(store, rules)
	_ = assessEngine.Run(ctx, assessmentID)
	findings, _ := store.GetFindingsByAssessment(assessmentID)
	sp5.succeed(fmt.Sprintf("Security: %d findings", len(findings)))

	// Step 6: Sizing.
	sp6 := newSpinner("Analyzing infrastructure sizing...")
	sizingAnalyzer := sizing.NewAnalyzer(store)
	sizingSummary, err := sizingAnalyzer.Analyze(assessmentID)
	if err != nil {
		sp6.fail("Sizing unavailable")
	} else {
		sp6.succeed(fmt.Sprintf("Sizing: %d vCPUs, %.1f GB memory", sizingSummary.TotalVCPUs, sizingSummary.TotalMemoryGB))
	}

	// Step 7: Cost.
	sp7 := newSpinner("Estimating costs...")
	costEstimator := cost.NewEstimator(store)
	costSummary, err := costEstimator.Estimate(assessmentID)
	if err != nil {
		sp7.fail("Cost estimation unavailable")
	} else {
		sp7.succeed(fmt.Sprintf("Estimated $%.2f/month", costSummary.TotalMonthlyCost))
	}

	// Step 8: Checklist.
	sp8 := newSpinner("Generating checklist...")
	checkEngine := checklist.NewEngine(store)
	checkSummary, err := checkEngine.Generate(assessmentID)
	if err != nil {
		sp8.fail("Checklist unavailable")
	} else {
		sp8.succeed(fmt.Sprintf("Checklist: %d pass, %d fail, %d warn", checkSummary.PassCount, checkSummary.FailCount, checkSummary.WarnCount))
	}

	// Done.
	completedAt := time.Now()
	_ = store.UpdateAssessmentStatus(assessmentID, "completed", &completedAt)
	elapsed := time.Since(now).Round(time.Millisecond)

	fmt.Println()
	fmt.Printf("  %s  %s\n\n", doneStyle.Render("✓ Assessment complete"), dimStyle.Render(elapsed.String()))

	if noTUI {
		return nil
	}

	fmt.Printf("  %s\n\n", dimStyle.Render("Launching TUI... (q to quit)"))
	time.Sleep(300 * time.Millisecond)

	model := tui.NewModel(store, assessmentID)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func resolveDBPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func canaryTableForProvider(providerType string) string {
	switch providerType {
	case "aws":
		return "aws_account"
	case "oci":
		return "oci_identity_compartment"
	default:
		return "unknown"
	}
}
