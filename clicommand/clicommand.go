// Package clicommand provides shared implementations of CLI accounts, keys, and admin-key subcommands.
package clicommand

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/terastal-hq/proxylib/accounts"
	"github.com/terastal-hq/proxylib/apikey"
)

func printLine(w io.Writer, a ...any)                  { _, _ = fmt.Fprintln(w, a...) }
func printFormat(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// Config defines the environment and callbacks needed by generic CLI commands.
type Config[A accounts.MutableAccount] struct {
	AppName        string
	Output         io.Writer
	GetAccounts    func() ([]A, error)
	UpdateAccounts func(func(*[]A) error) error
	KeyPrefix      string
	GenerateKey    func() (string, error)
	GenerateKeyID  func() (string, error)
}

// RunAccounts executes the `accounts <list|enable|disable|remove> [ID]` subcommand.
func RunAccounts[A accounts.MutableAccount](args []string, cfg Config[A]) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s accounts <list|enable|disable|remove> [ID]", cfg.AppName)
	}

	switch args[0] {
	case "list":
		accs, err := cfg.GetAccounts()
		if err != nil {
			return err
		}
		if len(accs) == 0 {
			printLine(cfg.Output, "No accounts configured.")
			return nil
		}
		printLine(cfg.Output, "CONFIGURED ACCOUNTS:")
		for _, acc := range accs {
			statusStr := "enabled"
			if !acc.IsEnabled() {
				statusStr = "disabled"
			}
			email := acc.GetEmail()
			if email == "" {
				email = "(not identified)"
			}
			printFormat(cfg.Output, "  • %-16s [%s] email: %-24s keys: %d\n", acc.GetID(), statusStr, email, len(acc.GetKeys()))
		}
		return nil

	case "enable", "disable":
		if len(args) < 2 {
			return fmt.Errorf("usage: %s accounts %s <account-id>", cfg.AppName, args[0])
		}
		targetID := strings.TrimSpace(args[1])
		enableVal := args[0] == "enable"

		found := false
		err := cfg.UpdateAccounts(func(accList *[]A) error {
			for i := range *accList {
				if (*accList)[i].GetID() == targetID {
					found = true
					(*accList)[i].SetEnabled(enableVal)
					return nil
				}
			}
			return fmt.Errorf("account %q not found", targetID)
		})
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("account %q not found", targetID)
		}
		printFormat(cfg.Output, "Account %q %sd.\n", targetID, args[0])
		printLine(cfg.Output, "Note: If the proxy server is running, restart it to load configuration changes.")
		return nil

	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: %s accounts remove <account-id>", cfg.AppName)
		}
		targetID := strings.TrimSpace(args[1])
		found := false
		err := cfg.UpdateAccounts(func(accList *[]A) error {
			filtered := make([]A, 0, len(*accList))
			for _, acc := range *accList {
				if acc.GetID() == targetID {
					found = true
					continue
				}
				filtered = append(filtered, acc)
			}
			if !found {
				return fmt.Errorf("account %q not found", targetID)
			}
			*accList = filtered
			return nil
		})
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("account %q not found", targetID)
		}
		printFormat(cfg.Output, "Account %q removed.\n", targetID)
		printLine(cfg.Output, "Note: If the proxy server is running, restart it to load configuration changes.")
		return nil

	default:
		return fmt.Errorf("unknown accounts command %q (valid: list, enable, disable, remove)", args[0])
	}
}

// RunKeys executes the `keys <list|create|revoke> <ACCOUNT_ID> [KEY_ID] [--label LABEL]` subcommand.
func RunKeys[A accounts.MutableAccount](args []string, cfg Config[A]) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s keys <list|create|revoke> <ACCOUNT_ID> [KEY_ID] [--label LABEL]", cfg.AppName)
	}

	switch args[0] {
	case "list":
		if len(args) < 2 {
			return fmt.Errorf("usage: %s keys list <account-id>", cfg.AppName)
		}
		accountID := strings.TrimSpace(args[1])
		accs, err := cfg.GetAccounts()
		if err != nil {
			return err
		}
		var target A
		var found bool
		for _, a := range accs {
			if a.GetID() == accountID {
				target = a
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("account %q not found", accountID)
		}
		keys := target.GetKeys()
		if len(keys) == 0 {
			printFormat(cfg.Output, "No API keys configured for account %q.\n", accountID)
			return nil
		}
		printFormat(cfg.Output, "API KEYS FOR ACCOUNT %q:\n", accountID)
		printFormat(cfg.Output, "  %-16s %-20s %-8s %s\n", "KEY ID", "LABEL", "LAST4", "CREATED (UTC)")
		for _, k := range keys {
			lbl := k.Label
			if lbl == "" {
				lbl = "-"
			}
			printFormat(cfg.Output, "  %-16s %-20s %-8s %s\n", k.ID, lbl, k.Last4, k.CreatedAt.Format(time.RFC3339))
		}
		return nil

	case "create":
		if len(args) < 2 {
			return fmt.Errorf("usage: %s keys create <account-id> [--label LABEL]", cfg.AppName)
		}
		accountID := strings.TrimSpace(args[1])
		fs := flag.NewFlagSet("keys create", flag.ContinueOnError)
		fs.SetOutput(cfg.Output)
		label := fs.String("label", "", "Label for the API key")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}

		genKey := cfg.GenerateKey
		if genKey == nil {
			genKey = func() (string, error) {
				return apikey.Generate(cfg.KeyPrefix)
			}
		}
		genID := cfg.GenerateKeyID
		if genID == nil {
			genID = apikey.GenerateID
		}

		rawSecret, err := genKey()
		if err != nil {
			return fmt.Errorf("generate key: %w", err)
		}
		keyID, err := genID()
		if err != nil {
			return fmt.Errorf("generate key id: %w", err)
		}

		keyObj := accounts.Key{
			ID:        keyID,
			Label:     strings.TrimSpace(*label),
			Hash:      apikey.Hash(rawSecret),
			Last4:     apikey.Last4(rawSecret),
			CreatedAt: time.Now().UTC(),
		}

		found := false
		err = cfg.UpdateAccounts(func(accList *[]A) error {
			for i := range *accList {
				if (*accList)[i].GetID() == accountID {
					found = true
					(*accList)[i].AddKey(keyObj)
					return nil
				}
			}
			return fmt.Errorf("account %q not found", accountID)
		})
		if !found {
			return fmt.Errorf("account %q not found", accountID)
		}
		if err != nil {
			return err
		}

		printFormat(cfg.Output, "Created API key for account %q:\n", accountID)
		printFormat(cfg.Output, "  Key:   %s\n", rawSecret)
		printFormat(cfg.Output, "  ID:    %s\n", keyID)
		if keyObj.Label != "" {
			printFormat(cfg.Output, "  Label: %s\n", keyObj.Label)
		}
		printLine(cfg.Output, "\nSave this secret key now - it cannot be displayed again.")
		printLine(cfg.Output, "Note: If the proxy server is running, restart it to load configuration changes.")
		return nil

	case "revoke":
		if len(args) < 3 {
			return fmt.Errorf("usage: %s keys revoke <account-id> <key-id>", cfg.AppName)
		}
		accountID := strings.TrimSpace(args[1])
		keyID := strings.TrimSpace(args[2])

		foundAccount := false
		foundKey := false
		err := cfg.UpdateAccounts(func(accList *[]A) error {
			for i := range *accList {
				if (*accList)[i].GetID() == accountID {
					foundAccount = true
					if (*accList)[i].RemoveKey(keyID) {
						foundKey = true
						return nil
					}
					return fmt.Errorf("key %q not found in account %q", keyID, accountID)
				}
			}
			return fmt.Errorf("account %q not found", accountID)
		})
		if err != nil {
			return err
		}
		if !foundAccount {
			return fmt.Errorf("account %q not found", accountID)
		}
		if !foundKey {
			return fmt.Errorf("key %q not found in account %q", keyID, accountID)
		}

		printFormat(cfg.Output, "Key %q revoked from account %q.\n", keyID, accountID)
		printLine(cfg.Output, "Note: If the proxy server is running, restart it to load configuration changes.")
		return nil

	default:
		return fmt.Errorf("unknown keys command %q (valid: list, create, revoke)", args[0])
	}
}

// RunAdminKey executes the `admin-key rotate` subcommand.
func RunAdminKey(args []string, appName string, output io.Writer, updateHash func(hash string) error, generateKey func() (string, error)) error {
	if len(args) == 0 || args[0] != "rotate" {
		return fmt.Errorf("usage: %s admin-key rotate", appName)
	}

	if generateKey == nil {
		generateKey = func() (string, error) {
			return apikey.Generate("")
		}
	}

	secret, err := generateKey()
	if err != nil {
		return fmt.Errorf("generate admin key: %w", err)
	}

	hash := apikey.Hash(secret)
	if err := updateHash(hash); err != nil {
		return fmt.Errorf("save admin key: %w", err)
	}

	printLine(output, "Rotated admin API key:")
	printFormat(output, "  Key: %s\n\n", secret)
	printLine(output, "Save this key now - it cannot be displayed again.")
	printLine(output, "Note: If the proxy server is running, restart it to apply the new admin key.")
	return nil
}
