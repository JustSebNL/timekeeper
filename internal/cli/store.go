package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JustSebNL/timekeeper/internal/storeclient"
)

// storeBaseURL resolves the Store gateway. The local application never
// requires it: with no URL configured every command fails cleanly and
// core TimeKeeper keeps working.
func storeBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("TIMEKEEPER_STORE_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return storeclient.DefaultBaseURL
}

// storeStateDir resolves where the installation identity and the
// short-lived session live. Order: explicit override, the repository's
// .timekeeper directory, then a per-user fallback.
func storeStateDir() string {
	if v := strings.TrimSpace(os.Getenv("TIMEKEEPER_STORE_STATE_DIR")); v != "" {
		return v
	}
	if root := storeRepoRoot(); root != "" {
		return filepath.Join(root, ".timekeeper")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".timekeeper"
	}
	return filepath.Join(home, ".timekeeper")
}

// storeRepoRoot walks up from the working directory for the repository
// markers, matching the rest of the CLI's path resolution.
func storeRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "install.sh")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

// newStoreClient builds a Store client. An explicit baseURL (from
// --url or the resolved environment) wins; otherwise the store-specific
// environment and defaults apply.
func newStoreClient(baseURL string) (*storeclient.Client, error) {
	if strings.TrimSpace(baseURL) != "" {
		return storeclient.New(strings.TrimSuffix(strings.TrimSpace(baseURL), "/"), storeStateDir())
	}
	return storeclient.New(storeBaseURL(), storeStateDir())
}

// storeCommand dispatches the `tk store` group. Named storeCommand
// because the cli package already imports internal/store as `store`.
func storeCommand(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) == 0 {
		return storeHelp(errOut)
	}
	switch args[0] {
	case "status":
		return storeStatus(out, errOut, baseURL)
	case "login":
		return storeLogin(args[1:], out, errOut, baseURL)
	case "register":
		return storeRegister(args[1:], out, errOut, baseURL)
	case "logout":
		return storeLogout(out, errOut, baseURL)
	case "whoami":
		return storeWhoami(out, errOut, baseURL)
	case "catalogue", "catalog":
		return storeCatalogue(out, errOut, baseURL)
	case "claim":
		return storeClaim(args[1:], out, errOut, baseURL)
	case "install":
		return storeInstall(args[1:], out, errOut, baseURL)
	case "list":
		return storeEntitlements(out, errOut, baseURL)
	case "devices":
		return storeDevices(out, errOut, baseURL)
	case "key":
		return storeKey(out, errOut, baseURL)
	case "help":
		return storeHelp(out)
	default:
		_, _ = fmt.Fprintf(errOut, "unknown store command %q\n", args[0])
		return storeHelp(errOut)
	}
}

func storeHelp(out io.Writer) int {
	_, _ = fmt.Fprint(out, `Usage: tk store <command>

Store access is optional. Core TimeKeeper works with the Store unreachable.

  status                     Show installation id and login state
  login <email>              Authenticate (--password or TIMEKEEPER_STORE_PASSWORD)
  logout                     Revoke this session and clear local credentials
  whoami                     Show the authenticated account
  catalogue                  List Store extensions
  claim <product-id>         Claim a free extension
  install <product-id> [ver] Verify, download and install an extension
  list                       Show this account's entitlements
  devices                    Show installations registered to this account
  key                        Show the Store package verification key

The Store decides access. This client only sends requests and verifies
package signatures.
`)
	return 0
}

func storeStatus(out, errOut io.Writer, baseURL string) int {
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "store url:       %s\n", client.BaseURL)
	_, _ = fmt.Fprintf(out, "installation id: %s\n", client.State.InstallationID)
	_, _ = fmt.Fprintf(out, "state file:      %s\n", filepath.Join(storeStateDir(), storeclient.StateFileName))
	if client.IsAuthenticated() {
		_, _ = fmt.Fprintf(out, "account:         %s (%s)\n", client.State.AccountPublicID, client.State.Email)
		_, _ = fmt.Fprintf(out, "logged in:       yes\n")
	} else {
		_, _ = fmt.Fprintf(out, "logged in:       no (run: tk store login <email>)\n")
	}
	return 0
}

func storeLogin(args []string, out, errOut io.Writer, baseURL string) int {
	email, password, _, ok := parseStoreCredentials(args, errOut)
	if !ok {
		return 2
	}

	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	account, err := client.Login(email, password)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "login failed: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "logged in as %s (%s)\n", account.PublicID, account.Email)
	return 0
}

// parseStoreCredentials accepts --password on the command line or
// TIMEKEEPER_STORE_PASSWORD from the environment. There is no silent
// interactive prompt: echoing a password into a terminal scrollback is
// worse than requiring a flag.
func parseStoreCredentials(args []string, errOut io.Writer) (email, password, displayName string, ok bool) {
	password = os.Getenv("TIMEKEEPER_STORE_PASSWORD")
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--password" && i+1 < len(args):
			i++
			password = args[i]
		case strings.HasPrefix(args[i], "--password="):
			password = strings.TrimPrefix(args[i], "--password=")
		case args[i] == "--name" && i+1 < len(args):
			i++
			displayName = args[i]
		case strings.HasPrefix(args[i], "--name="):
			displayName = strings.TrimPrefix(args[i], "--name=")
		case strings.HasPrefix(args[i], "--"):
			_, _ = fmt.Fprintf(errOut, "unknown flag %q\n", args[i])
			return "", "", "", false
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) > 0 {
		email = strings.TrimSpace(positional[0])
	}
	if email == "" || password == "" {
		_, _ = fmt.Fprint(errOut, "usage: tk store login|register <email> --password <password>\n"+
			"       (or set TIMEKEEPER_STORE_PASSWORD; the password is never echoed or stored)\n")
		return "", "", "", false
	}
	return email, password, displayName, true
}

func storeRegister(args []string, out, errOut io.Writer, baseURL string) int {
	email, password, displayName, ok := parseStoreCredentials(args, errOut)
	if !ok {
		return 2
	}
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	account, err := client.Register(email, password, displayName)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "registration failed: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "registered %s (%s)\n", account.PublicID, account.Email)
	return 0
}

func storeLogout(out, errOut io.Writer, baseURL string) int {
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	if err := client.Logout(); err != nil {
		_, _ = fmt.Fprintf(errOut, "logout failed: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(out, "logged out")
	return 0
}

func storeWhoami(out, errOut io.Writer, baseURL string) int {
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	account, err := client.Account()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	return writeJSON(out, account)
}

func storeCatalogue(out, errOut io.Writer, baseURL string) int {
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	catalogue, err := client.Catalogue()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	if len(catalogue.Extensions) == 0 {
		_, _ = fmt.Fprintln(out, "no extensions published")
		return 0
	}
	for _, e := range catalogue.Extensions {
		state := "available"
		if !e.Available {
			state = "locked"
		}
		price := "free"
		if e.PriceCents > 0 {
			price = fmt.Sprintf("%s %d.%02d", e.Currency, e.PriceCents/100, e.PriceCents%100)
		}
		_, _ = fmt.Fprintf(out, "%-34s %-10s %-8s %s\n", e.ProductID, e.LatestVersion, price, state)
	}
	return 0
}

func storeClaim(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(errOut, "usage: tk store claim <product-id>")
		return 2
	}
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	claimed, err := client.Claim(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "claim failed: %v\n", err)
		return 1
	}
	if claimed.AlreadyOwned {
		_, _ = fmt.Fprintf(out, "%s already claimed\n", claimed.ProductID)
		return 0
	}
	_, _ = fmt.Fprintf(out, "claimed %s\n", claimed.ProductID)
	return 0
}

func storeEntitlements(out, errOut io.Writer, baseURL string) int {
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	list, err := client.Entitlements()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no entitlements")
		return 0
	}
	for _, e := range list {
		state := "active"
		if !e.Active {
			state = "inactive"
		}
		_, _ = fmt.Fprintf(out, "%-34s %-12s %s\n", e.ProductID, e.Source, state)
	}
	return 0
}

func storeDevices(out, errOut io.Writer, baseURL string) int {
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	devices, err := client.Devices()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	for _, d := range devices {
		label := d.Label
		if label == "" {
			label = "-"
		}
		_, _ = fmt.Fprintf(out, "%-28s %-20s %s\n", d.InstallationID, label, d.LastSeenAt)
	}
	return 0
}

func storeKey(out, errOut io.Writer, baseURL string) int {
	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	key, err := client.PublicKey()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "algorithm:   %s\n", key.Algorithm)
	_, _ = fmt.Fprintf(out, "fingerprint: %s\n", key.Fingerprint)
	_, _ = fmt.Fprintf(out, "signed:      %s\n", key.SignedObject)
	_, _ = fmt.Fprintln(out, key.PublicKey)
	return 0
}

// storeInstall walks the whole delivery path: install request, ticket
// download, signature verification, then extraction. The package is
// never extracted before its signature verifies.
func storeInstall(args []string, out, errOut io.Writer, baseURL string) int {
	if len(args) < 1 || len(args) > 2 {
		_, _ = fmt.Fprintln(errOut, "usage: tk store install <product-id> [version]")
		return 2
	}
	productID := strings.TrimSpace(args[0])
	version := ""
	if len(args) == 2 {
		version = strings.TrimSpace(args[1])
	}

	client, err := newStoreClient(baseURL)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "store: %v\n", err)
		return 1
	}

	result, err := client.Install(productID, version)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "install failed: %v\n", err)
		return 1
	}

	key, err := client.PublicKey()
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "cannot fetch the verification key: %v\n", err)
		return 1
	}

	body, headers, err := client.DownloadPackage(result.Download.TicketID)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "download failed: %v\n", err)
		return 1
	}
	defer body.Close()

	staging := filepath.Join(storeStateDir(), "extensions", ".staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		_, _ = fmt.Fprintf(errOut, "create staging dir: %v\n", err)
		return 1
	}
	archivePath := filepath.Join(staging, result.ProductID+"-"+result.Version+".tkp")
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "write package: %v\n", err)
		return 1
	}
	if _, err := io.Copy(f, body); err != nil {
		f.Close()
		_, _ = fmt.Fprintf(errOut, "write package: %v\n", err)
		return 1
	}
	f.Close()

	sha := result.SHA256
	if headers != nil && headers.SHA256 != "" {
		sha = headers.SHA256
	}
	signature := result.Signature
	if headers != nil && headers.Signature != "" {
		signature = headers.Signature
	}

	// Verify before anything is unpacked. A bad signature leaves
	// nothing behind but the rejected archive.
	if err := storeclient.VerifyPackage(archivePath, sha, signature, key.PublicKey); err != nil {
		_ = os.Remove(archivePath)
		_, _ = fmt.Fprintf(errOut, "package rejected: %v\n", err)
		return 1
	}

	dest := filepath.Join(storeStateDir(), "extensions", result.ProductID)
	manifest, err := storeclient.ExtractPackage(archivePath, dest)
	if err != nil {
		_ = os.Remove(archivePath)
		_, _ = fmt.Fprintf(errOut, "extract failed: %v\n", err)
		return 1
	}
	_ = os.Remove(archivePath)

	_, _ = fmt.Fprintf(out, "installed %s %s\n", manifest.ID, manifest.Version)
	_, _ = fmt.Fprintf(out, "path:        %s\n", dest)
	if len(manifest.Permissions) > 0 {
		_, _ = fmt.Fprintf(out, "permissions: %s\n", strings.Join(manifest.Permissions, ", "))
		_, _ = fmt.Fprintln(out, "TimeKeeper Core grants these; the package cannot widen them itself.")
	}
	return 0
}

func writeJSON(out io.Writer, value interface{}) int {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(out, "%+v\n", value)
		return 0
	}
	_, _ = fmt.Fprintln(out, string(encoded))
	return 0
}
