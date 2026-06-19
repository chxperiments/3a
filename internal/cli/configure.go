package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chxmxii/a3/internal/config"
)

func newConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Interactive setup wizard for A3 profiles",
		Long:  "Interactively configure a new cloud provider profile, credentials, and Steampipe connection.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigure()
		},
	}
	return cmd
}

func runConfigure() error {
	reader := bufio.NewReader(os.Stdin)

	// Step 1: Provider.
	fmt.Print("Provider? (aws/oci): ")
	provider, err := readLine(reader)
	if err != nil {
		return err
	}
	provider = strings.ToLower(provider)
	if provider != "aws" && provider != "oci" {
		return fmt.Errorf("unsupported provider: %s (must be aws or oci)", provider)
	}

	// Step 2: Profile name.
	fmt.Print("Profile name for A3: ")
	profileName, err := readLine(reader)
	if err != nil {
		return err
	}
	if profileName == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	// Step 3: Credentials.
	var awsProfile string
	var ociProfile string
	switch provider {
	case "aws":
		awsProfile, err = configureAWSCredentials(reader, profileName)
		if err != nil {
			return err
		}
	case "oci":
		ociProfile, err = configureOCICredentials(reader)
		if err != nil {
			return err
		}
	}

	// Step 4: Regions.
	defaultRegion := "us-east-1"
	if provider == "oci" {
		// OCI region identifiers differ from AWS; "*" lets the plugin use every
		// region subscribed by the tenancy's home region.
		defaultRegion = "*"
	}
	fmt.Printf("Regions? (comma-separated, * for all, default %s): ", defaultRegion)
	regionsInput, err := readLine(reader)
	if err != nil {
		return err
	}
	var regions []string
	if regionsInput == "" {
		regions = []string{defaultRegion}
	} else if strings.TrimSpace(regionsInput) == "*" {
		regions = []string{"*"}
	} else {
		for _, r := range strings.Split(regionsInput, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				regions = append(regions, r)
			}
		}
	}
	if len(regions) == 0 {
		regions = []string{defaultRegion}
	}

	// Step 5: Write Steampipe connection config.
	switch provider {
	case "aws":
		if err := writeSteampipeAWSConfig(profileName, awsProfile, regions); err != nil {
			return fmt.Errorf("writing Steampipe config: %w", err)
		}
	case "oci":
		if err := writeSteampipeOCIConfig(profileName, ociProfile, regions); err != nil {
			return fmt.Errorf("writing Steampipe config: %w", err)
		}
	}

	// Step 6: Write A3 profile to ~/.a3/config.yaml.
	if err := writeA3Profile(profileName, provider, awsProfile, ociProfile, regions); err != nil {
		return fmt.Errorf("writing A3 config: %w", err)
	}

	// Step 7: Ensure Steampipe and the provider plugin are installed (each step
	// asks for confirmation before changing anything on the system).
	if err := ensureSteampipeInstalled(reader); err != nil {
		return err
	}
	if err := ensureSteampipePlugin(reader, provider); err != nil {
		return err
	}
	ensureSteampipeService(reader)

	// Step 8: Summary.
	spcFile := "aws.spc"
	connPrefix := "aws"
	if provider == "oci" {
		spcFile = "oci.spc"
		connPrefix = "oci"
	}
	fmt.Println()
	fmt.Println("✓ Configuration complete!")
	fmt.Println()
	fmt.Printf("  Profile:    %s\n", profileName)
	fmt.Printf("  Provider:   %s\n", provider)
	if provider == "aws" {
		fmt.Printf("  AWS Profile: %s\n", awsProfile)
	} else {
		fmt.Printf("  OCI Profile: %s\n", ociProfile)
	}
	fmt.Printf("  Regions:    %s\n", strings.Join(regions, ", "))
	fmt.Println()
	fmt.Printf("  Steampipe:  ~/.steampipe/config/%s (connection \"%s_%s\")\n", spcFile, connPrefix, profileName)
	fmt.Printf("  A3 Config:  ~/.a3/config.yaml\n")
	fmt.Println()
	fmt.Printf("  Run: a3 assess %s\n", profileName)

	return nil
}

// configureAWSCredentials prompts for the AWS credential method and returns the
// AWS profile name Steampipe should use.
func configureAWSCredentials(reader *bufio.Reader, profileName string) (string, error) {
	fmt.Print("Credential method? (1=SSO profile, 2=Access keys, 3=Existing AWS profile): ")
	method, err := readLine(reader)
	if err != nil {
		return "", err
	}

	switch method {
	case "1":
		// SSO profile.
		fmt.Print("SSO profile name to use: ")
		ssoProfile, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if ssoProfile == "" {
			return "", fmt.Errorf("SSO profile name cannot be empty")
		}
		return ssoProfile, nil

	case "2":
		// Access keys — write to ~/.aws/credentials.
		fmt.Print("AWS_ACCESS_KEY_ID: ")
		accessKey, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if accessKey == "" {
			return "", fmt.Errorf("access key cannot be empty")
		}

		fmt.Print("AWS_SECRET_ACCESS_KEY: ")
		secretKey, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if secretKey == "" {
			return "", fmt.Errorf("secret key cannot be empty")
		}

		if err := writeAWSCredentials(profileName, accessKey, secretKey); err != nil {
			return "", fmt.Errorf("writing AWS credentials: %w", err)
		}
		return profileName, nil

	case "3":
		// Existing AWS profile.
		fmt.Print("Existing AWS profile name from ~/.aws/credentials: ")
		existingProfile, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if existingProfile == "" {
			return "", fmt.Errorf("profile name cannot be empty")
		}
		return existingProfile, nil

	default:
		return "", fmt.Errorf("invalid credential method: %s (must be 1, 2, or 3)", method)
	}
}

// configureOCICredentials prompts for the OCI config profile Steampipe should
// use. Credentials themselves live in ~/.oci/config (set up via `oci setup
// config`); the OCI Steampipe plugin reads that profile directly.
func configureOCICredentials(reader *bufio.Reader) (string, error) {
	fmt.Print("OCI config profile name from ~/.oci/config (default DEFAULT): ")
	ociProfile, err := readLine(reader)
	if err != nil {
		return "", err
	}
	if ociProfile == "" {
		ociProfile = "DEFAULT"
	}

	// Warn (but don't fail) if ~/.oci/config is missing — the user may set it
	// up before running an assessment.
	if home, herr := os.UserHomeDir(); herr == nil {
		ociConfig := filepath.Join(home, ".oci", "config")
		if _, serr := os.Stat(ociConfig); serr != nil {
			fmt.Printf("  ⚠ %s not found — set up OCI credentials with 'oci setup config' before running an assessment.\n", ociConfig)
		}
	}

	return ociProfile, nil
}

// readLine reads a line from the reader and trims whitespace.
func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptYesNo asks a yes/no question and returns the answer. An empty response
// returns def.
func promptYesNo(reader *bufio.Reader, question string, def bool) bool {
	suffix := "[Y/n]"
	if !def {
		suffix = "[y/N]"
	}
	fmt.Printf("%s %s ", question, suffix)
	line, err := readLine(reader)
	if err != nil {
		return def
	}
	line = strings.ToLower(line)
	if line == "" {
		return def
	}
	return line == "y" || line == "yes"
}

// commandExists reports whether name is found on the PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ensureSteampipeInstalled checks that the steampipe binary is available and, if
// not, offers to install it via the official installer (with confirmation).
func ensureSteampipeInstalled(reader *bufio.Reader) error {
	if commandExists("steampipe") {
		return nil
	}

	fmt.Println()
	fmt.Println("Steampipe is not installed. A3 uses Steampipe to query cloud resources.")
	if !promptYesNo(reader, "Install Steampipe now?", true) {
		return fmt.Errorf("Steampipe is required — install it from https://steampipe.io/downloads and re-run 'a3 configure'")
	}

	fmt.Println("Installing Steampipe (you may be prompted for your password)...")
	cmd := exec.Command("sh", "-c", "curl -fsSL https://steampipe.io/install/steampipe.sh | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("steampipe installation failed: %w\nInstall it manually from https://steampipe.io/downloads and re-run 'a3 configure'", err)
	}
	if !commandExists("steampipe") {
		return fmt.Errorf("steampipe was installed but is not on your PATH — open a new shell (or add it to PATH) and re-run 'a3 configure'")
	}
	return nil
}

// steampipePluginInstalled reports whether the turbot plugin for the given
// provider appears to be installed under ~/.steampipe/plugins.
func steampipePluginInstalled(plugin string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	path := filepath.Join(home, ".steampipe", "plugins", "hub.steampipe.io", "plugins", "turbot", plugin+"@latest")
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ensureSteampipePlugin checks that the Steampipe plugin for the given provider
// is installed and, if not, offers to install it (with confirmation).
func ensureSteampipePlugin(reader *bufio.Reader, plugin string) error {
	if !commandExists("steampipe") {
		// ensureSteampipeInstalled already gates on this; nothing to do.
		return nil
	}
	if steampipePluginInstalled(plugin) {
		return nil
	}

	fmt.Println()
	fmt.Printf("The Steampipe %q plugin is not installed.\n", plugin)
	if !promptYesNo(reader, fmt.Sprintf("Install the %q plugin now?", plugin), true) {
		fmt.Printf("  Skipping — install it later with: steampipe plugin install %s\n", plugin)
		return nil
	}

	fmt.Printf("Installing Steampipe %q plugin...\n", plugin)
	cmd := exec.Command("steampipe", "plugin", "install", plugin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("plugin install failed: %w", err)
	}
	return nil
}

// ensureSteampipeService offers to start the Steampipe service so `a3 assess`
// can connect. Best-effort: failure to start is reported but not fatal.
func ensureSteampipeService(reader *bufio.Reader) {
	if !commandExists("steampipe") {
		return
	}
	fmt.Println()
	if !promptYesNo(reader, "Start the Steampipe service now (required for 'a3 assess')?", true) {
		fmt.Println("  Skipping — start it later with: steampipe service start")
		return
	}
	cmd := exec.Command("steampipe", "service", "start")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("  ⚠ Could not start Steampipe service: %v\n  Start it manually with: steampipe service start\n", err)
	}
}

// writeAWSCredentials appends a profile section to ~/.aws/credentials.
func writeAWSCredentials(profileName, accessKey, secretKey string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	credDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(credDir, 0o755); err != nil {
		return fmt.Errorf("creating ~/.aws directory: %w", err)
	}

	credFile := filepath.Join(credDir, "credentials")

	block := fmt.Sprintf("\n[%s]\naws_access_key_id = %s\naws_secret_access_key = %s\n",
		profileName, accessKey, secretKey)

	f, err := os.OpenFile(credFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening credentials file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(block); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}

	fmt.Printf("  ✓ Wrote credentials to %s\n", credFile)
	return nil
}

// formatRegions renders a Steampipe HCL region list, e.g. ["us-east-1", "eu-west-1"].
func formatRegions(regions []string) string {
	if len(regions) == 1 && regions[0] == "*" {
		return "[\"*\"]"
	}
	quoted := make([]string, len(regions))
	for i, r := range regions {
		quoted[i] = fmt.Sprintf("%q", r)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// writeSteampipeAWSConfig appends a connection block to ~/.steampipe/config/aws.spc.
func writeSteampipeAWSConfig(profileName, awsProfile string, regions []string) error {
	spcFile, err := steampipeConfigPath("aws.spc")
	if err != nil {
		return err
	}

	block := fmt.Sprintf("\nconnection \"aws_%s\" {\n  plugin  = \"aws\"\n  profile = \"%s\"\n  regions = %s\n}\n",
		profileName, awsProfile, formatRegions(regions))

	return appendSteampipeBlock(spcFile, block)
}

// writeSteampipeOCIConfig appends a connection block to ~/.steampipe/config/oci.spc.
// The OCI plugin reads credentials from the named ~/.oci/config profile via
// config_file_profile. "*" regions are omitted so the plugin falls back to the
// tenancy's subscribed regions rather than receiving an invalid literal.
func writeSteampipeOCIConfig(profileName, ociProfile string, regions []string) error {
	spcFile, err := steampipeConfigPath("oci.spc")
	if err != nil {
		return err
	}

	var regionsLine string
	if !(len(regions) == 1 && regions[0] == "*") {
		regionsLine = fmt.Sprintf("\n  regions             = %s", formatRegions(regions))
	}

	block := fmt.Sprintf("\nconnection \"oci_%s\" {\n  plugin              = \"oci\"\n  config_file_profile = \"%s\"%s\n}\n",
		profileName, ociProfile, regionsLine)

	return appendSteampipeBlock(spcFile, block)
}

// steampipeConfigPath returns the path to a file in ~/.steampipe/config,
// creating the directory if needed.
func steampipeConfigPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	spcDir := filepath.Join(home, ".steampipe", "config")
	if err := os.MkdirAll(spcDir, 0o755); err != nil {
		return "", fmt.Errorf("creating steampipe config directory: %w", err)
	}
	return filepath.Join(spcDir, name), nil
}

// appendSteampipeBlock appends an HCL connection block to a .spc file.
func appendSteampipeBlock(spcFile, block string) error {
	f, err := os.OpenFile(spcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening steampipe config: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(block); err != nil {
		return fmt.Errorf("writing steampipe config: %w", err)
	}

	fmt.Printf("  ✓ Wrote Steampipe connection to %s\n", spcFile)
	return nil
}

// writeA3Profile writes or updates the A3 profile in ~/.a3/config.yaml.
func writeA3Profile(profileName, provider, awsProfile, ociProfile string, regions []string) error {
	if _, err := config.EnsureConfigDir(); err != nil {
		return err
	}

	cfgPath := config.DefaultConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		// If config doesn't exist, create a new one.
		cfg = &config.Config{
			DBPath: "~/.a3/a3.db",
			Steampipe: config.SteampipeConfig{
				ConnectionString: "postgres://steampipe@localhost:9193/steampipe",
			},
		}
	}

	profile := config.AccountProfile{
		Name:     profileName,
		Provider: provider,
		Regions:  regions,
	}
	switch provider {
	case "aws":
		profile.AwsProfile = awsProfile
	case "oci":
		profile.OciProfile = ociProfile
	}

	// Check if profile already exists and update it, or add new.
	found := false
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == profileName {
			cfg.Profiles[i] = profile
			found = true
			break
		}
	}
	if !found {
		config.AddProfile(cfg, profile)
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}

	fmt.Printf("  ✓ Wrote A3 profile to %s\n", cfgPath)
	return nil
}
