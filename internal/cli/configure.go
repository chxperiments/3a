package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chxmxii/3a/internal/config"
)

func newConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Interactive setup wizard for 3A profiles",
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
	fmt.Print("Profile name for 3A: ")
	profileName, err := readLine(reader)
	if err != nil {
		return err
	}
	if profileName == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	// Step 3: Credential method (AWS-specific).
	var awsProfile string
	if provider == "aws" {
		fmt.Print("Credential method? (1=SSO profile, 2=Access keys, 3=Existing AWS profile): ")
		method, err := readLine(reader)
		if err != nil {
			return err
		}

		switch method {
		case "1":
			// SSO profile.
			fmt.Print("SSO profile name to use: ")
			ssoProfile, err := readLine(reader)
			if err != nil {
				return err
			}
			if ssoProfile == "" {
				return fmt.Errorf("SSO profile name cannot be empty")
			}
			awsProfile = ssoProfile

		case "2":
			// Access keys — write to ~/.aws/credentials.
			fmt.Print("AWS_ACCESS_KEY_ID: ")
			accessKey, err := readLine(reader)
			if err != nil {
				return err
			}
			if accessKey == "" {
				return fmt.Errorf("access key cannot be empty")
			}

			fmt.Print("AWS_SECRET_ACCESS_KEY: ")
			secretKey, err := readLine(reader)
			if err != nil {
				return err
			}
			if secretKey == "" {
				return fmt.Errorf("secret key cannot be empty")
			}

			if err := writeAWSCredentials(profileName, accessKey, secretKey); err != nil {
				return fmt.Errorf("writing AWS credentials: %w", err)
			}
			awsProfile = profileName

		case "3":
			// Existing AWS profile.
			fmt.Print("Existing AWS profile name from ~/.aws/credentials: ")
			existingProfile, err := readLine(reader)
			if err != nil {
				return err
			}
			if existingProfile == "" {
				return fmt.Errorf("profile name cannot be empty")
			}
			awsProfile = existingProfile

		default:
			return fmt.Errorf("invalid credential method: %s (must be 1, 2, or 3)", method)
		}
	}

	// Step 7: Regions.
	fmt.Print("Regions? (comma-separated, * for all, default us-east-1): ")
	regionsInput, err := readLine(reader)
	if err != nil {
		return err
	}
	var regions []string
	if regionsInput == "" {
		regions = []string{"us-east-1"}
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
		regions = []string{"us-east-1"}
	}

	// Step 8: Write Steampipe connection config.
	if provider == "aws" {
		if err := writeSteampipeAWSConfig(profileName, awsProfile, regions); err != nil {
			return fmt.Errorf("writing Steampipe config: %w", err)
		}
	}

	// Step 9: Write 3A profile to ~/.3a/config.yaml.
	if err := write3AProfile(profileName, provider, awsProfile, regions); err != nil {
		return fmt.Errorf("writing 3A config: %w", err)
	}

	// Step 10: Summary.
	fmt.Println()
	fmt.Println("✓ Configuration complete!")
	fmt.Println()
	fmt.Printf("  Profile:    %s\n", profileName)
	fmt.Printf("  Provider:   %s\n", provider)
	if provider == "aws" {
		fmt.Printf("  AWS Profile: %s\n", awsProfile)
	}
	fmt.Printf("  Regions:    %s\n", strings.Join(regions, ", "))
	fmt.Println()
	fmt.Printf("  Steampipe:  ~/.steampipe/config/aws.spc (connection \"aws_%s\")\n", profileName)
	fmt.Printf("  3A Config:  ~/.3a/config.yaml\n")
	fmt.Println()
	fmt.Printf("  Run: 3a assess %s\n", profileName)

	return nil
}

// readLine reads a line from the reader and trims whitespace.
func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
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

// writeSteampipeAWSConfig appends a connection block to ~/.steampipe/config/aws.spc.
func writeSteampipeAWSConfig(profileName, awsProfile string, regions []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	spcDir := filepath.Join(home, ".steampipe", "config")
	if err := os.MkdirAll(spcDir, 0o755); err != nil {
		return fmt.Errorf("creating steampipe config directory: %w", err)
	}

	spcFile := filepath.Join(spcDir, "aws.spc")

	// Build regions list.
	var regionsList string
	if len(regions) == 1 && regions[0] == "*" {
		regionsList = "[\"*\"]"
	} else {
		quotedRegions := make([]string, len(regions))
		for i, r := range regions {
			quotedRegions[i] = fmt.Sprintf("%q", r)
		}
		regionsList = "[" + strings.Join(quotedRegions, ", ") + "]"
	}

	block := fmt.Sprintf("\nconnection \"aws_%s\" {\n  plugin  = \"aws\"\n  profile = \"%s\"\n  regions = %s\n}\n",
		profileName, awsProfile, regionsList)

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

// write3AProfile writes or updates the 3A profile in ~/.3a/config.yaml.
func write3AProfile(profileName, provider, awsProfile string, regions []string) error {
	if _, err := config.EnsureConfigDir(); err != nil {
		return err
	}

	cfgPath := config.DefaultConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		// If config doesn't exist, create a new one.
		cfg = &config.Config{
			DBPath: "~/.3a/3a.db",
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
	if provider == "aws" {
		profile.AwsProfile = awsProfile
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

	fmt.Printf("  ✓ Wrote 3A profile to %s\n", cfgPath)
	return nil
}
