package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

func newAdminCommand(rt *runtimeConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Operator utilities for TrustDB admin web",
	}
	cmd.AddCommand(newAdminHashPasswordCommand())
	return cmd
}

func newAdminHashPasswordCommand() *cobra.Command {
	var pwd string
	cmd := &cobra.Command{
		Use:   "hash-password",
		Short: "Print a bcrypt hash suitable for TRUSTDB_ADMIN_PASSWORD_HASH",
		RunE: func(cmd *cobra.Command, args []string) error {
			secret := strings.TrimSpace(pwd)
			if secret == "" {
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "password: ")
				line, err := bufio.NewReader(os.Stdin).ReadString('\n')
				if err != nil {
					return err
				}
				secret = strings.TrimSpace(line)
			}
			if secret == "" {
				return usageError("password is required")
			}
			out, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	cmd.Flags().StringVar(&pwd, "password", "", "plaintext password (omit to read one line from stdin)")
	return cmd
}
