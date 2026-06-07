package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mallardduck/dirio/common/output"
	"github.com/mallardduck/dirio/cmd/client/internal/dioclient/profile"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage dio client profiles",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create or update a profile in ~/.dirio/client.yaml",
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the active profile configuration",
	RunE:  runConfigShow,
}

var configProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List all configured profiles",
	RunE:  runConfigProfiles,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configProfilesCmd)
}

func runConfigInit(_ *cobra.Command, _ []string) error {
	cfg, err := profile.Load()
	if err != nil {
		return err
	}

	// Pre-fill from an existing profile if the user supplies --profile.
	profileName := flagProfile
	if profileName == "" {
		profileName = "local"
	}
	existing := cfg.Profiles[profileName]

	var (
		name      = profileName
		endpoint  = existing.Endpoint
		accessKey = existing.AccessKey
		secretKey = existing.SecretKey
		region    = existing.Region
		setDef    bool
	)
	if endpoint == "" {
		endpoint = "http://localhost:9000"
	}
	if region == "" {
		region = "us-east-1"
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Profile name").
				Description("Identifier for this server connection (e.g. local, prod)").
				Value(&name),
			huh.NewInput().
				Title("Endpoint").
				Description("S3 API base URL (e.g. http://localhost:9000)").
				Value(&endpoint),
			huh.NewInput().
				Title("Access key").
				Value(&accessKey),
			huh.NewInput().
				Title("Secret key").
				EchoMode(huh.EchoModePassword).
				Value(&secretKey),
			huh.NewInput().
				Title("Region").
				Value(&region),
			huh.NewConfirm().
				Title("Set as default profile?").
				Value(&setDef),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("config init: %w", err)
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]profile.Profile)
	}
	cfg.Profiles[name] = profile.Profile{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Region:    region,
	}
	if setDef || cfg.DefaultProfile == "" {
		cfg.DefaultProfile = name
	}

	if err := profile.Save(cfg); err != nil {
		return err
	}

	path, _ := profile.ConfigPath()
	output.Success(fmt.Sprintf("Saved profile %q to %s", name, path))
	if cfg.DefaultProfile == name {
		output.Hint("This is now your default profile.")
	}
	output.Blank()
	return nil
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := profile.Load()
	if err != nil {
		return err
	}

	resolved := profile.Resolve(cfg, flagProfile)

	profileName := flagProfile
	if profileName == "" {
		profileName = cfg.DefaultProfile
	}
	if profileName == "" {
		profileName = "(none)"
	}

	output.Header("Active profile: " + profileName)
	output.Blank()
	output.Field("Endpoint", resolved.Endpoint)
	output.Field("Access key", resolved.AccessKey)
	output.Field("Region", resolved.Region)
	output.Blank()
	output.Hint("Secret key is not displayed.")
	output.Blank()
	return nil
}

func runConfigProfiles(_ *cobra.Command, _ []string) error {
	cfg, err := profile.Load()
	if err != nil {
		return err
	}

	if len(cfg.Profiles) == 0 {
		fmt.Fprintln(os.Stderr, "No profiles configured. Run: dio config init")
		return nil
	}

	// Print a simple YAML block per profile for readability.
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	fmt.Print(string(out))
	return nil
}
