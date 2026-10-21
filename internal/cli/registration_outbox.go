package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/instanceguard"
)

const (
	registrationOutboxPending      = "Pending"
	registrationOutboxAwaitingCore = "AwaitingCore"
	registrationOutboxRegistered   = "Registered"
	registrationOutboxRejected     = "Rejected"
)

var registrationOutboxIDPattern = regexp.MustCompile(`^OQ-[0-9a-f]{32}$`)
var registrationOutboxPathPattern = regexp.MustCompile(`^/api/v1/[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*$`)

func isSafeRegistrationOutboxPath(path string) bool {
	return registrationOutboxPathPattern.MatchString(path)
}

type registrationOutboxOffer struct {
	LocalID        string    `json:"local_id"`
	ProjectID      string    `json:"project_id,omitempty"`
	Kind           string    `json:"kind"`
	IdempotencyKey string    `json:"idempotency_key"`
	ValidationPath string    `json:"validation_path,omitempty"`
	SubmitPath     string    `json:"submit_path"`
	Body           []byte    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

type registrationOutboxState struct {
	Version       int             `json:"version"`
	Status        string          `json:"status"`
	CoreQueueID   string          `json:"core_queue_id,omitempty"`
	CoreProjectID string          `json:"core_project_id,omitempty"`
	Attempts      int             `json:"attempts"`
	LastError     string          `json:"last_error,omitempty"`
	Validation    json.RawMessage `json:"validation,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type registrationOutboxEntry struct {
	Offer registrationOutboxOffer `json:"offer"`
	State registrationOutboxState `json:"state"`
}

func acquireRegistrationOutboxLock(path string) (*instanceguard.Lock, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		lock, err := instanceguard.Acquire(path)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, instanceguard.ErrAlreadyRunning) {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("timed out waiting for registration outbox lock %s: %w", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func registrationOutboxRoot() string {
	return filepath.Join(storeStateDir(), "registration-outbox")
}

func registrationOutboxItemDir(localID string) string {
	if !registrationOutboxIDPattern.MatchString(localID) {
		return ""
	}
	return filepath.Join(registrationOutboxRoot(), "items", localID)
}

func enqueueRegistrationOutbox(projectID, kind, validationPath, submitPath string, body []byte) (registrationOutboxEntry, error) {
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(submitPath) == "" {
		return registrationOutboxEntry{}, errors.New("registration outbox kind and submit path are required")
	}
	if !isSafeRegistrationOutboxPath(strings.TrimSpace(submitPath)) || (strings.TrimSpace(validationPath) != "" && !isSafeRegistrationOutboxPath(strings.TrimSpace(validationPath))) {
		return registrationOutboxEntry{}, errors.New("registration outbox paths must be relative Core API routes")
	}
	if !json.Valid(body) {
		return registrationOutboxEntry{}, errors.New("registration outbox payload must be valid JSON")
	}
	localID, err := newRegistrationOutboxID()
	if err != nil {
		return registrationOutboxEntry{}, err
	}
	if err := ensureRegistrationOutboxRoot(); err != nil {
		return registrationOutboxEntry{}, err
	}
	queueLock, err := acquireRegistrationOutboxLock(filepath.Join(registrationOutboxRoot(), "enqueue"))
	if err != nil {
		return registrationOutboxEntry{}, fmt.Errorf("lock registration outbox FIFO: %w", err)
	}
	defer queueLock.Close()
	entries, err := listRegistrationOutbox()
	if err != nil {
		return registrationOutboxEntry{}, err
	}
	createdAt := time.Now().UTC()
	if len(entries) > 0 && !createdAt.After(entries[len(entries)-1].Offer.CreatedAt) {
		createdAt = entries[len(entries)-1].Offer.CreatedAt.Add(time.Nanosecond)
	}
	itemDir := registrationOutboxItemDir(localID)
	if itemDir == "" {
		return registrationOutboxEntry{}, errors.New("invalid generated registration outbox ID")
	}
	if err := ensureStoreDirectory(itemDir); err != nil {
		return registrationOutboxEntry{}, err
	}
	if err := os.Chmod(itemDir, 0o700); err != nil {
		return registrationOutboxEntry{}, fmt.Errorf("secure registration outbox item directory: %w", err)
	}

	offer := registrationOutboxOffer{
		LocalID:        localID,
		ProjectID:      strings.TrimSpace(projectID),
		Kind:           strings.TrimSpace(kind),
		IdempotencyKey: strings.TrimPrefix(localID, "OQ-") + "-timekeeper",
		ValidationPath: strings.TrimSpace(validationPath),
		SubmitPath:     strings.TrimSpace(submitPath),
		Body:           append([]byte(nil), body...),
		CreatedAt:      createdAt,
	}
	if err := writeRegistrationOutboxFile(filepath.Join(itemDir, "offer.json"), offer); err != nil {
		return registrationOutboxEntry{}, fmt.Errorf("persist registration offer %s: %w", localID, err)
	}
	state := registrationOutboxState{Version: 1, Status: registrationOutboxPending, UpdatedAt: createdAt}
	if err := writeRegistrationOutboxState(itemDir, state); err != nil {
		// The immutable offer remains discoverable. Readers treat a missing state
		// log as Pending, so a crash here cannot erase the submitted payload.
		return registrationOutboxEntry{}, fmt.Errorf("persist initial state for offer %s: %w", localID, err)
	}
	return registrationOutboxEntry{Offer: offer, State: state}, nil
}

func newRegistrationOutboxID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate registration outbox ID: %w", err)
	}
	return "OQ-" + hex.EncodeToString(raw[:]), nil
}

func ensureRegistrationOutboxRoot() error {
	root := registrationOutboxRoot()
	if err := ensureStoreDirectory(root); err != nil {
		return fmt.Errorf("create registration outbox root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return fmt.Errorf("secure registration outbox root: %w", err)
	}
	itemsDir := filepath.Join(root, "items")
	if err := ensureStoreDirectory(itemsDir); err != nil {
		return fmt.Errorf("create registration outbox items: %w", err)
	}
	return os.Chmod(itemsDir, 0o700)
}

func writeRegistrationOutboxFile(path string, value any) error {
	parent := filepath.Dir(path)
	if err := ensureStoreDirectory(parent); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(parent, ".offer-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func writeRegistrationOutboxState(itemDir string, state registrationOutboxState) error {
	stateDir := filepath.Join(itemDir, "state")
	if err := ensureStoreDirectory(stateDir); err != nil {
		return err
	}
	if err := os.Chmod(stateDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%020d.json", state.Version)
	finalPath := filepath.Join(stateDir, name)
	temp, err := os.CreateTemp(stateDir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, finalPath)
}

func appendRegistrationOutboxState(localID string, next registrationOutboxState) error {
	entry, err := loadRegistrationOutboxEntry(localID)
	if err != nil {
		return err
	}
	switch next.Status {
	case registrationOutboxPending, registrationOutboxAwaitingCore, registrationOutboxRegistered, registrationOutboxRejected:
	default:
		return fmt.Errorf("invalid registration outbox status %q", next.Status)
	}
	next.Version = entry.State.Version + 1
	if next.UpdatedAt.IsZero() {
		next.UpdatedAt = time.Now().UTC()
	} else {
		next.UpdatedAt = next.UpdatedAt.UTC()
	}
	return writeRegistrationOutboxState(registrationOutboxItemDir(localID), next)
}

func listRegistrationOutbox() ([]registrationOutboxEntry, error) {
	if err := ensureRegistrationOutboxRoot(); err != nil {
		return nil, err
	}
	itemsDir := filepath.Join(registrationOutboxRoot(), "items")
	entries, err := os.ReadDir(itemsDir)
	if os.IsNotExist(err) {
		return []registrationOutboxEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list registration outbox: %w", err)
	}
	out := make([]registrationOutboxEntry, 0, len(entries))
	for _, item := range entries {
		if !item.IsDir() || !registrationOutboxIDPattern.MatchString(item.Name()) {
			continue
		}
		entry, err := loadRegistrationOutboxEntry(item.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Offer.CreatedAt.Equal(out[j].Offer.CreatedAt) {
			return out[i].Offer.LocalID < out[j].Offer.LocalID
		}
		return out[i].Offer.CreatedAt.Before(out[j].Offer.CreatedAt)
	})
	return out, nil
}

func loadRegistrationOutboxEntry(localID string) (registrationOutboxEntry, error) {
	if err := ensureRegistrationOutboxRoot(); err != nil {
		return registrationOutboxEntry{}, err
	}
	itemDir := registrationOutboxItemDir(localID)
	if itemDir == "" {
		return registrationOutboxEntry{}, fmt.Errorf("invalid registration outbox ID %q", localID)
	}
	if err := requireRegistrationOutboxDirectory(itemDir); err != nil {
		return registrationOutboxEntry{}, err
	}
	offerPath := filepath.Join(itemDir, "offer.json")
	if err := requireRegistrationOutboxRegularFile(offerPath); err != nil {
		return registrationOutboxEntry{}, err
	}
	data, err := os.ReadFile(offerPath)
	if err != nil {
		return registrationOutboxEntry{}, fmt.Errorf("read registration offer %s: %w", localID, err)
	}
	var offer registrationOutboxOffer
	if err := json.Unmarshal(data, &offer); err != nil {
		return registrationOutboxEntry{}, fmt.Errorf("decode registration offer %s: %w", localID, err)
	}
	if offer.LocalID != localID || !json.Valid(offer.Body) || strings.TrimSpace(offer.IdempotencyKey) == "" {
		return registrationOutboxEntry{}, fmt.Errorf("registration offer %s is invalid", localID)
	}
	state, err := readRegistrationOutboxState(itemDir)
	if err != nil {
		return registrationOutboxEntry{}, err
	}
	return registrationOutboxEntry{Offer: offer, State: state}, nil
}

func requireRegistrationOutboxDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect registration outbox directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("registration outbox path %s is not a regular directory", path)
	}
	return nil
}

func requireRegistrationOutboxRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect registration outbox file %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("registration outbox path %s is not a regular file", path)
	}
	return nil
}

func readRegistrationOutboxState(itemDir string) (registrationOutboxState, error) {
	stateDir := filepath.Join(itemDir, "state")
	if err := requireRegistrationOutboxDirectory(stateDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return registrationOutboxState{Status: registrationOutboxPending}, nil
		}
		return registrationOutboxState{}, err
	}
	files, err := os.ReadDir(stateDir)
	if err != nil {
		return registrationOutboxState{}, fmt.Errorf("list registration outbox state: %w", err)
	}
	latest := -1
	var chosen string
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		version, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil || version < 1 || version <= latest {
			continue
		}
		latest, chosen = version, filepath.Join(stateDir, file.Name())
	}
	if chosen == "" {
		return registrationOutboxState{Status: registrationOutboxPending}, nil
	}
	if err := requireRegistrationOutboxRegularFile(chosen); err != nil {
		return registrationOutboxState{}, err
	}
	data, err := os.ReadFile(chosen)
	if err != nil {
		return registrationOutboxState{}, fmt.Errorf("read registration outbox state: %w", err)
	}
	var state registrationOutboxState
	if err := json.Unmarshal(data, &state); err != nil {
		return registrationOutboxState{}, fmt.Errorf("decode registration outbox state %s: %w", chosen, err)
	}
	if state.Version != latest {
		return registrationOutboxState{}, fmt.Errorf("registration outbox state version mismatch in %s", chosen)
	}
	return state, nil
}
