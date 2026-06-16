package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chxmxii/3a/internal/config"
)

func newProfilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "Manage assessment profiles",
	}

	cmd.AddCommand(newProfilesListCmd())
	cmd.AddCommand(newProfilesAddCmd())

	return cmd
}

func newProfilesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := config.DefaultConfigPath()
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			profiles := config.ListProfiles(cfg)
			if len(profiles) == 0 {
				fmt.Println("No profiles configured. Use '3a profiles add' to create one.")
				return nil
			}

			fmt.Printf("%-20s %-10s %-30s\n", "NAME", "PROVIDER", "REGIONS")
			fmt.Printf("%-20s %-10s %-30s\n", "----", "--------", "-------")
			for _, p := range profiles {
				regions := "all"
				if len(p.Regions) > 0 {
					regions = fmt.Sprintf("%v", p.Regions)
				}
				fmt.Printf("%-20s %-10s %-30s\n", p.Name, p.Provider, regions)
			}
			return nil
		},
	}
}

func newProfilesAddCmd() *cobra.Command {
	var provider string
	var regions []string
	var displayName string

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfgPath := config.DefaultConfigPath()
			cfg, err := config.Load(cfgPath)
			if err != nil {
				// Create new config if it doesn't exist.
				cfg = &config.Config{
					DBPath: "~/.3a/3a.db",
				}
			}

			// Check for duplicates.
			for _, p := range cfg.Profiles {
				if p.Name == name {
					return fmt.Errorf("profile %q already exists", name)
				}
			}

			if displayName == "" {
				displayName = name
			}

			profile := config.AccountProfile{
				Name:        name,
				DisplayName: displayName,
				Provider:    provider,
				Regions:     regions,
			}

			config.AddProfile(cfg, profile)

			if err := config.Save(cfgPath, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("Profile %q added successfully.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "aws", "cloud provider (aws or oci)")
	cmd.Flags().StringSliceVar(&regions, "regions", []string{"us-east-1"}, "regions to assess")
	cmd.Flags().StringVar(&displayName, "display-name", "", "display name for the profile")

	return cmd
}
