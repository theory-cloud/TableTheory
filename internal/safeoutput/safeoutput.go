// Package safeoutput writes generated and scaffolded files through an
// os.Root-confined, atomic rename so that a symlink in the destination path
// cannot redirect a write to an unrelated victim file outside its intended
// directory.
package safeoutput

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	filePerm     = 0o600
	rootDirPerm  = 0o750
	outDirPerm   = 0o700
	tempAttempts = 10
)

// WriteFileWithin writes data to relPath beneath root, creating missing parent
// directories with mode 0o750. relPath must be relative and must not escape
// root. Parent components are resolved with root confinement: a symlinked
// parent that resolves outside root is refused, while a symlink that stays
// within root is followed. A symlink at the final component is replaced rather
// than followed. The file is written to a temporary file in the destination
// directory and renamed into place.
//
// root must already exist and is treated as the containment anchor; callers
// that must also reject a symlinked root should check that beforehand.
func WriteFileWithin(root, relPath string, data []byte, perm fs.FileMode) (err error) {
	if root == "" {
		return errors.New("safeoutput: empty root")
	}
	rel, err := cleanRel(relPath)
	if err != nil {
		return err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := handle.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	return writeFileAtomic(handle, rel, data, perm, rootDirPerm)
}

// WriteFileAtomic writes data to path, creating missing parent directories with
// mode 0o700. It refuses a symlinked parent component that resolves outside the
// nearest real ancestor directory of path, and replaces rather than follows a
// symlink at the final component. The file is written to a temporary file in
// the destination directory and renamed into place.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	if path == "" {
		return errors.New("safeoutput: empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	clean := filepath.Clean(abs)
	anchor, err := nearestRealDir(filepath.Dir(clean))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(anchor, clean)
	if err != nil {
		return err
	}
	if rel == "." {
		return fmt.Errorf("safeoutput: %s is a directory", path)
	}
	rel, err = cleanRel(rel)
	if err != nil {
		return err
	}
	handle, err := os.OpenRoot(anchor)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := handle.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	return writeFileAtomic(handle, rel, data, perm, outDirPerm)
}

func cleanRel(relPath string) (string, error) {
	if relPath == "" {
		return "", errors.New("safeoutput: empty path")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("safeoutput: path %q must be relative", relPath)
	}
	clean := filepath.Clean(relPath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("safeoutput: path %q escapes root", relPath)
	}
	return clean, nil
}

// nearestRealDir returns the deepest ancestor of dir (or dir itself) that
// exists as a directory reached without following a symlink. Symlinked
// components are skipped so the returned anchor is a real directory; a later
// confined write then refuses any skipped component that resolves outside it.
func nearestRealDir(dir string) (string, error) {
	current := filepath.Clean(dir)
	for {
		info, err := os.Lstat(current)
		switch {
		case err == nil && info.IsDir():
			return current, nil
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return current, nil
		}
		current = parent
	}
}

func writeFileAtomic(root *os.Root, rel string, data []byte, perm, dirPerm fs.FileMode) (err error) {
	dir := filepath.Dir(rel)
	if dir != "." {
		if mkdirErr := root.MkdirAll(dir, dirPerm); mkdirErr != nil {
			return mkdirErr
		}
	}
	tmpName, file, err := createTemp(root, dir, filepath.Base(rel))
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}
		if err != nil {
			if removeErr := root.Remove(tmpName); removeErr != nil {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if perm != filePerm {
		if err = file.Chmod(perm); err != nil {
			return err
		}
	}
	if err = file.Close(); err != nil {
		return err
	}
	closed = true
	return root.Rename(tmpName, rel)
}

func createTemp(root *os.Root, dir, base string) (string, *os.File, error) {
	for range tempAttempts {
		suffix, err := randomSuffix()
		if err != nil {
			return "", nil, err
		}
		name := filepath.Join(dir, "."+base+".tmp-"+suffix)
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
		if err == nil {
			return name, file, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("safeoutput: cannot create temporary file for %s", filepath.Join(dir, base))
}

func randomSuffix() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
