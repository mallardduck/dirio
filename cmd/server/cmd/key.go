package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mallardduck/dirio/internal/config"
	"github.com/mallardduck/dirio/internal/crypto"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage the DirIO encryption key",
	Long: `Commands for generating and rotating the credential encryption key.

DirIO encrypts secret keys at rest using AES-256-GCM. The encryption key is
stored in <data-dir>/.dirio/keyring (one "base64:<key>" per line). The first
line is the active key; subsequent lines are previous keys kept as decryption
fallbacks during rotation.

For production deployments, override the keyring with the
DIRIO_ENCRYPTION_KEY environment variable (and DIRIO_PREVIOUS_ENCRYPTION_KEYS
for rotation fallbacks).`,
}

var keyGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new encryption key and print it to stdout",
	Long: `Generate a new random 32-byte encryption key and print it to stdout.

The key is printed in "base64:<encoded>" format. You can use it by:

  • Setting it as DIRIO_ENCRYPTION_KEY for production deployments.
  • Pasting it as the first line of <data-dir>/.dirio/keyring.
  • Running "dirio key rotate" to do the keyring update automatically.`,
	RunE: runKeyGenerate,
}

var keyRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Rotate the encryption key in the keyring file",
	Long: `Generate a new encryption key and prepend it to the keyring file.

The current key is preserved as a fallback so all previously encrypted values
remain readable after rotation. Restart dirio for the new key to take effect —
new writes will use the new key while old values decrypt via the fallback list.

Once you are confident that all credentials have been re-encrypted with the new
key (e.g. after a "dirio rekey" run), you can safely remove old key lines from
the keyring file.`,
	RunE: runKeyRotate,
}

func init() {
	rootCmd.AddCommand(keyCmd)
	keyCmd.AddCommand(keyGenerateCmd)
	keyCmd.AddCommand(keyRotateCmd)

	// rotate needs the data directory to find the keyring file.
	keyRotateCmd.Flags().StringP(
		config.DataDir.GetFlagKey(), "d",
		config.DataDir.GetDefaultAsString(),
		"Path to data directory (must match the running server)",
	)
}

func runKeyGenerate(_ *cobra.Command, _ []string) error {
	key, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}
	fmt.Println(key)
	return nil
}

func runKeyRotate(cmd *cobra.Command, _ []string) error {
	dataDir, _ := cmd.Flags().GetString(config.DataDir.GetFlagKey())
	if dataDir == "" {
		dataDir = config.DataDir.GetDefaultAsString()
	}

	newKey, err := crypto.RotateKeyring(dataDir)
	if err != nil {
		return fmt.Errorf("failed to rotate keyring: %w", err)
	}

	keyringPath := dataDir + "/.dirio/keyring"

	fmt.Fprintf(os.Stderr, "\n[dirio] Encryption key rotated successfully.\n")
	fmt.Fprintf(os.Stderr, "[dirio] New key:     %s\n", newKey)
	fmt.Fprintf(os.Stderr, "[dirio] Keyring:     %s\n", keyringPath)
	fmt.Fprintf(os.Stderr, "[dirio] Next steps:\n")
	fmt.Fprintf(os.Stderr, "[dirio]   1. Restart dirio — new writes will use the new key.\n")
	fmt.Fprintf(os.Stderr, "[dirio]   2. Old values will continue to decrypt via the fallback list.\n")
	fmt.Fprintf(os.Stderr, "[dirio]   3. Once all credentials are re-encrypted, remove old key lines from the keyring.\n\n")

	return nil
}
