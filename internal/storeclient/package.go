package storeclient

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Package limits. A Store package is a small extension archive; these
// bounds stop a hostile or corrupt archive from filling the disk or
// exhausting memory during extraction.
const (
	MaxPackageFiles    = 500
	MaxPackageFileSize = 32 << 20 // 32 MB per file
	MaxPackageTotal    = 64 << 20 // 64 MB unpacked
)

// ExtractPackage unpacks a .tkp archive into destDir and returns its
// manifest.
//
// It refuses archives that try to escape the destination, that exceed
// the size bounds, or whose checksums.json does not match the files
// actually present. Callers must still call VerifyPackage on the
// archive before extraction: checksums inside an archive only prove
// internal consistency, not that the Store signed it.
func ExtractPackage(tkpPath, destDir string) (*Manifest, error) {
	f, err := os.Open(tkpPath)
	if err != nil {
		return nil, fmt.Errorf("open package: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("package is not gzip compressed: %w", err)
	}
	defer gz.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create extension dir: %w", err)
	}

	var total int64
	var files int
	sums := map[string]string{}

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read package: %w", err)
		}

		// Refuse on the raw name before normalising: an entry that
		// even mentions ".." is not part of the .tkp format.
		if strings.Contains(header.Name, "..") || strings.HasPrefix(header.Name, "/") {
			return nil, fmt.Errorf("package contains an unsafe path: %q", header.Name)
		}
		name := cleanArchiveName(header.Name)
		if name == "" {
			continue
		}
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
			return nil, fmt.Errorf("package contains an unsafe path: %q", header.Name)
		}

		target := filepath.Join(destDir, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("package escapes its destination: %q", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, fmt.Errorf("create dir %s: %w", name, err)
			}
			continue
		case tar.TypeReg:
		default:
			// Symlinks, devices and anything exotic are not part of the
			// .tkp format. Ignore rather than honour them.
			continue
		}

		files++
		if files > MaxPackageFiles {
			return nil, fmt.Errorf("package has more than %d files", MaxPackageFiles)
		}
		if header.Size > MaxPackageFileSize {
			return nil, fmt.Errorf("file %s exceeds the %d byte limit", name, MaxPackageFileSize)
		}
		total += header.Size
		if total > MaxPackageTotal {
			return nil, fmt.Errorf("package unpacks to more than %d bytes", MaxPackageTotal)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("create parent for %s: %w", name, err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
		hash := sha256.New()
		written, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(reader, MaxPackageFileSize+1))
		out.Close()
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
		if written > MaxPackageFileSize {
			return nil, fmt.Errorf("file %s exceeds the %d byte limit", name, MaxPackageFileSize)
		}
		sums[name] = hex.EncodeToString(hash.Sum(nil))
	}

	if err := verifyChecksums(destDir, sums); err != nil {
		return nil, err
	}
	return ReadManifest(destDir)
}

// cleanArchiveName normalises an entry name from the archive.
//
// Archive paths are always slash separated and may arrive as "./x" or
// "x". This uses the slash-based path package, never filepath: on
// Windows filepath.Clean rewrites separators and a harmless "./"
// entry turns into something that looks like an escape.
func cleanArchiveName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	if name == "." || name == "" {
		return ""
	}
	return name
}

// verifyChecksums compares the checksums.json shipped inside the
// package against the files that were actually written.
func verifyChecksums(destDir string, actual map[string]string) error {
	data, err := os.ReadFile(filepath.Join(destDir, "checksums.json"))
	if err != nil {
		return fmt.Errorf("package is missing checksums.json: %w", err)
	}
	var declared map[string]string
	if err := json.Unmarshal(data, &declared); err != nil {
		return fmt.Errorf("package checksums.json is invalid: %w", err)
	}
	for name, want := range declared {
		got, ok := actual[cleanArchiveName(name)]
		if !ok {
			return fmt.Errorf("package is missing a declared file: %s", name)
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", name, want, got)
		}
	}
	return nil
}

// ReadManifest loads and validates manifest.json from an extracted
// package. Permissions are returned verbatim; TimeKeeper Core decides
// what to grant, never the package itself.
func ReadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("package is missing manifest.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("package manifest.json is invalid: %w", err)
	}
	if strings.TrimSpace(manifest.ID) == "" {
		return nil, fmt.Errorf("package manifest has no id")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return nil, fmt.Errorf("package manifest has no version")
	}
	return &manifest, nil
}
