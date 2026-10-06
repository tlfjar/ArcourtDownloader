package arcourt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// All operations after opening the chosen directory are relative to pinned
// directory handles. Never construct an absolute court-derived path for I/O.
func openOutputDirectory(path string) (*os.Root, error) {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return nil, ErrUnsafeDestination
	}
	volume := filepath.VolumeName(path)
	parts := strings.FieldsFunc(strings.TrimPrefix(path, volume), func(r rune) bool {
		return r == '/' || (filepath.Separator == '\\' && r == '\\')
	})
	for _, p := range parts {
		if p == ".." || strings.TrimRight(p, " .") != p || strings.Contains(p, ":") {
			return nil, ErrUnsafeDestination
		}
	}
	r, err := os.OpenRoot(volume + string(filepath.Separator))
	if err != nil {
		return nil, err
	}
	for _, p := range parts {
		if p == "." {
			continue
		}
		next, err := openDirectory(r, p)
		r.Close()
		if err != nil {
			return nil, err
		}
		r = next
	}
	return r, nil
}

func openDirectory(parent *os.Root, name string) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() || isReparse(before) {
		return nil, ErrUnsafeDestination
	}
	r, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := r.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		r.Close()
		return nil, ErrUnsafeDestination
	}
	return r, nil
}

func safeComponent(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 160 &&
		!strings.ContainsAny(name, `/\:`) && strings.TrimRight(name, " .") == name && filepath.IsLocal(name)
}

// Reject symlinks/reparse points and non-regular files before opening, and verify
// the opened identity. Root also prevents a concurrent replacement escaping it.
func openRegular(root *os.Root, name string, flags int) (*os.File, error) {
	if !safeComponent(name) {
		return nil, ErrUnsafeDestination
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || isReparse(before) {
		return nil, ErrUnsafeDestination
	}
	f, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		f.Close()
		return nil, ErrUnsafeDestination
	}
	return f, nil
}

type downloadDisk struct {
	root *os.Root
	// Private deterministic failure injection; no permissions or full-disk tricks.
	before func(operation, name string) error
}

func (d *downloadDisk) check(op, name string) error {
	if d.before != nil {
		return d.before(op, name)
	}
	return nil
}

func (d *downloadDisk) temp(kind string) (*os.File, string, error) {
	if err := d.check(kind+".create", ""); err != nil {
		return nil, "", err
	}
	for range 10 {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, "", err
		}
		name := ".arcourt-" + kind + "-" + hex.EncodeToString(nonce[:]) + ".tmp"
		f, err := d.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, name, err
	}
	return nil, "", errors.New("could not allocate a unique temporary file")
}

func (d *downloadDisk) remove(name string) error {
	if name == "" {
		return nil
	}
	if err := d.check("remove", name); err != nil {
		return err
	}
	err := d.root.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (d *downloadDisk) writeClosed(kind string, data []byte) (name string, err error) {
	f, name, err := d.temp(kind)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, d.remove(name))
		}
	}()
	err = d.check(kind+".write", name)
	if err == nil {
		var n int
		n, err = f.Write(data)
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = d.check(kind+".sync", name)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, d.check(kind+".close", name), f.Close())
	return name, err
}

func (d *downloadDisk) names() ([]string, error) {
	f, err := d.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}

func (d *downloadDisk) distinctName(base string, records []LocalDocumentResult) (string, error) {
	names, err := d.names()
	if err != nil {
		return "", err
	}
	used := make(map[string]bool, len(names))
	for _, n := range names {
		used[strings.ToLower(n)] = true
	}
	for _, r := range records {
		used[strings.ToLower(r.Filename)] = true
	}
	for i := 0; i < 100000; i++ {
		name := base
		if i > 0 {
			name = strings.TrimSuffix(base, ".pdf") + fmt.Sprintf("-%d.pdf", i)
		}
		if !used[strings.ToLower(name)] {
			return name, nil
		}
	}
	return "", errors.New("too many filename collisions")
}

func lockOutput(root *os.Root) (*os.File, error) {
	const name = ".arcourt.lock"
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrExist) {
		f, err = openRegular(root, name, os.O_RDWR)
	}
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	// Never unlink lock files: another process may already hold the same inode.
	return f, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
