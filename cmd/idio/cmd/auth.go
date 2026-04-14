// SPDX-License-Identifier: Apache-2.0

// auth.go — Hidden placeholder for future platform authentication commands.
//
//   - All subcommands (login, logout, status, whoami) are Hidden from --help.
//   - login and whoami are no-ops that print an informational message.
//   - logout and status use a JSON credentials file stored at
//     ~/.config/idio/credentials.json; the token helpers are defined here
//     so they can be shared when the feature is eventually implemented.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

func NewCmdAuth() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "auth",
		Short:  "Authenticate with the Idiomatic platform",
		Long:   "Login, logout, and manage authentication status.",
		Hidden: true,
	}

	cmd.AddCommand(newCmdAuthLogin())
	cmd.AddCommand(newCmdAuthLogout())
	cmd.AddCommand(newCmdAuthStatus())
	cmd.AddCommand(newCmdAuthWhoami())

	return cmd
}

func newCmdAuthLogin() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to Idiomatic",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Authentication is not available in the open-source release.")
			cmd.Println("Visit https://github.com/adamgilman/idiomatic for updates.")
			return nil
		},
	}
}

func newCmdAuthLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of Idiomatic",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := tokenPath()
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove credentials: %w", err)
			}
			cmd.Println("Logged out.")
			return nil
		},
	}
}

func newCmdAuthStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := loadToken()
			if err != nil {
				cmd.Println("Not authenticated. Run 'idio auth login' to sign in.")
				return nil
			}

			cmd.Printf("Authenticated.\nToken expires: %s\n", token.ExpiresAt.Format(time.RFC3339))
			return nil
		},
	}
}

func newCmdAuthWhoami() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated user",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Authentication is not available in the open-source release.")
			cmd.Println("Visit https://github.com/adamgilman/idiomatic for updates.")
			return nil
		},
	}
}

// Token storage.

type storedToken struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "idio")
	os.MkdirAll(dir, 0700)
	return dir
}

func tokenPath() string {
	return filepath.Join(configDir(), "credentials.json")
}

func loadToken() (*storedToken, error) {
	data, err := os.ReadFile(tokenPath())
	if err != nil {
		return nil, err
	}

	var token storedToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, fmt.Errorf("token expired")
	}

	return &token, nil
}
