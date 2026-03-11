package cmd

import (
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

var (
	openSecretsStore     = secrets.OpenDefault
	authorizeGoogle      = googleauth.Authorize
	startManageServer    = googleauth.StartManageServer
	checkRefreshToken    = googleauth.CheckRefreshToken
	ensureKeychainAccess = secrets.EnsureKeychainAccess
	fetchAuthorizedEmail = googleauth.EmailForRefreshToken
	manualAuthURL        = googleauth.ManualAuthURL
)

func ensureKeychainAccessIfNeeded() error {
	backendInfo, err := secrets.ResolveKeyringBackendInfo()
	if err != nil {
		return fmt.Errorf("resolve keyring backend: %w", err)
	}
	if backendInfo.Value == strFile {
		return nil
	}
	return ensureKeychainAccess()
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

const (
	authTypeOAuth               = "oauth"
	authTypeServiceAccount      = "service_account"
	authTypeOAuthServiceAccount = "oauth+service_account"
)


func parseAuthServices(servicesCSV string) ([]googleauth.Service, error) {
	trimmed := strings.ToLower(strings.TrimSpace(servicesCSV))
	if trimmed == "" || trimmed == "user" || trimmed == literalAll {
		return googleauth.UserServices(), nil
	}

	parts := strings.Split(servicesCSV, ",")
	seen := make(map[googleauth.Service]struct{})
	out := make([]googleauth.Service, 0, len(parts))
	for _, p := range parts {
		svc, err := googleauth.ParseService(p)
		if err != nil {
			return nil, err
		}
		if svc == googleauth.ServiceKeep {
			return nil, usage("Keep auth is Workspace-only and requires a service account. Use: gog auth service-account set <email> --key <service-account.json>")
		}
		if _, ok := seen[svc]; ok {
			continue
		}
		seen[svc] = struct{}{}
		out = append(out, svc)
	}

	return out, nil
}

func splitCommaList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make([]string, 0)
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t' || r == ' '
	})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}
