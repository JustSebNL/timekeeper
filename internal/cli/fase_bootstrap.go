package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

type trackedFASE struct {
	identity             model.FASEIdentity
	fingerprint, baseURL string
	resumeSession        string
}

var currentFASE atomic.Pointer[trackedFASE]
var trackedRunMu sync.Mutex

// RunTracked is the shipped executable boundary. Run remains the internal
// command dispatcher, allowing isolated command tests without fake enrollment.
func RunTracked(args []string, out, errOut io.Writer, baseURL string) int {
	trackedRunMu.Lock()
	defer trackedRunMu.Unlock()
	resumeAgent, resumeSession := strings.TrimSpace(os.Getenv("TIMEKEEPER_AGENT_ID")), strings.TrimSpace(os.Getenv("TIMEKEEPER_SESSION_ID"))
	var in model.FASEIdentityInput
	var err error
	var id model.FASEIdentity
	if strings.HasPrefix(resumeAgent, "FASE-") && resumeSession != "" {
		id, err = resolveRemoteFASEResume(baseURL, resumeAgent, resumeSession)
	} else {
		in, err = discoverFASEIdentity()
		if err == nil {
			id, err = resolveRemoteFASE(baseURL, in)
		}
	}
	if err != nil {
		fmt.Fprintf(errOut, "TimeKeeper identity unavailable: %v\n", err)
		normalized, _ := extractVerboseFlag(args)
		if len(normalized) == 0 {
			return 1
		}
		switch normalized[0] {
		case "help", "--help", "-h", "version", "doctor", "service", "hosts":
			return Run(args, out, errOut, baseURL)
		default:
			return 1
		}
	}
	trackedSession := ""
	if strings.HasPrefix(resumeAgent, "FASE-") {
		trackedSession = resumeSession
	}
	currentFASE.Store(&trackedFASE{identity: id, fingerprint: in.Fingerprint, resumeSession: trackedSession, baseURL: strings.TrimRight(baseURL, "/")})
	defer currentFASE.Store(nil)
	return Run(args, out, errOut, baseURL)
}

func discoverFASEIdentity() (model.FASEIdentityInput, error) {
	legacy := resolveAgentIdentity()
	runtimeName, scope, session := legacy.Runtime, "runtime-session", legacy.SessionID
	// Trusted wake adapters can carry the original fingerprint without rehashing.
	fingerprint := strings.TrimSpace(os.Getenv("TIMEKEEPER_FASE_FINGERPRINT"))
	if fingerprint != "" {
		if runtimeName == "" {
			runtimeName = "generic"
		}
		scope = strings.TrimSpace(os.Getenv("TIMEKEEPER_FASE_SCOPE"))
		if scope == "" {
			scope = "runtime-session"
		}
	} else {
		if session == "" {
			var err error
			session, err = processSessionSource()
			if err != nil {
				return model.FASEIdentityInput{}, err
			}
			runtimeName, scope = "process-host", "process-session"
		}
		encoded, _ := json.Marshal([]string{"fase-v1", runtimeName, scope, session})
		sum := sha256.Sum256(encoded)
		fingerprint = hex.EncodeToString(sum[:])
	}
	return model.FASEIdentityInput{Fingerprint: fingerprint, Runtime: runtimeName, IdentityScope: scope, Metadata: model.FASEMetadata{Provider: os.Getenv("TIMEKEEPER_MODEL_PROVIDER"), Model: os.Getenv("TIMEKEEPER_MODEL"), ModelVersion: os.Getenv("TIMEKEEPER_MODEL_VERSION"), RuntimeVersion: os.Getenv("TIMEKEEPER_RUNTIME_VERSION")}}, nil
}

func resolveRemoteFASE(baseURL string, in model.FASEIdentityInput) (model.FASEIdentity, error) {
	var id model.FASEIdentity
	body, err := json.Marshal(in)
	if err != nil {
		return id, err
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/agent-identities/resolve", bytes.NewReader(body))
	if err != nil {
		return id, err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return id, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		return id, fmt.Errorf("FASE handshake: %s; compatible Core required", formatAPIError(response.Status, data, response.Header.Get("X-Request-ID")))
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&id); err != nil {
		return id, err
	}
	if id.AgentKey == "" || id.AgentID != "FASE-"+id.AgentKey || !strings.HasPrefix(id.FASEID, "FASE-ID."+id.AgentKey+".") || id.Runtime != in.Runtime || id.IdentityScope != in.IdentityScope {
		return id, fmt.Errorf("Core returned an inconsistent FASE identity")
	}
	return id, nil
}

func resolveRemoteFASEResume(baseURL, agentID, sessionID string) (model.FASEIdentity, error) {
	body, err := json.Marshal(model.FASEResumeInput{AgentID: agentID, SessionID: sessionID})
	if err != nil {
		return model.FASEIdentity{}, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/agent-identities/resume", bytes.NewReader(body))
	if err != nil {
		return model.FASEIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return model.FASEIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return model.FASEIdentity{}, fmt.Errorf("FASE resume: %s", formatAPIError(resp.Status, data, resp.Header.Get("X-Request-ID")))
	}
	var id model.FASEIdentity
	if err = json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&id); err != nil {
		return id, err
	}
	if id.AgentID != agentID || id.AgentKey == "" {
		return id, fmt.Errorf("Core returned an inconsistent resumed FASE identity")
	}
	return id, nil
}

func sameFASEOrigin(endpoint *url.URL, base string) bool {
	origin, err := url.Parse(base)
	return err == nil && endpoint.Scheme == origin.Scheme && endpoint.Host == origin.Host
}
