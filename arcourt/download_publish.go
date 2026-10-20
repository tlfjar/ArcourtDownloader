package arcourt

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// A nil move keeps the existing hard-link publisher and its recovery witness.
// A move consumes its source only on success, and must never replace a target.
func (d *downloadDisk) publish(source, target string) error {
	if !safeComponent(source) || !safeComponent(target) || source == target {
		return ErrUnsafeDestination
	}
	var err error
	if d.move == nil {
		err = d.root.Link(source, target)
	} else {
		err = d.move(d.root, source, target)
	}
	return publicationError(err)
}

// Exercise the candidate on exclusively owned files before any court I/O.
// A filesystem label alone is not evidence of working no-replace semantics.
func (d *downloadDisk) probeMove() (err error) {
	source, err := d.writeClosed("probe", []byte("arcourt source"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, d.remove(source)) }()
	target, err := d.writeClosed("probe", []byte("arcourt target"))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, d.remove(target)) }()
	check := func(name, want string) bool {
		f, e := openRegular(d.root, name, os.O_RDONLY)
		if e != nil {
			return false
		}
		defer f.Close()
		data, e := io.ReadAll(io.LimitReader(f, 64))
		return e == nil && bytes.Equal(data, []byte(want))
	}
	collision := d.publish(source, target)
	if !errors.Is(collision, os.ErrExist) {
		if collision != nil {
			return collision
		}
		return ErrUnsupportedFilesystem
	}
	if !check(source, "arcourt source") || !check(target, "arcourt target") {
		return ErrUnsupportedFilesystem
	}
	if err = d.remove(target); err != nil {
		return err
	}
	if err = d.publish(source, target); err != nil {
		return err
	}
	if _, e := d.root.Lstat(source); !errors.Is(e, os.ErrNotExist) || !check(target, "arcourt source") {
		return ErrUnsupportedFilesystem
	}
	return nil
}

func filesystemFailure(err error) string {
	switch {
	case errors.Is(err, ErrRecoveryRequired):
		return "interrupted publication needs review; files were preserved. Use a new empty output folder, or review the receipt and its files before moving the receipt out of the case folder"
	case errors.Is(err, ErrOutputAccess):
		return "output access or file sharing blocked publication; check folder permissions and close other apps using these files"
	default:
		return "output filesystem cannot safely publish without overwriting; choose a writable local NTFS folder"
	}
}

// FilesystemErrorMessage returns controlled remediation text for shared storage
// errors so the CLI and desktop describe the same recovery and capability limits.
func FilesystemErrorMessage(err error) string {
	if errors.Is(err, ErrUnsupportedFilesystem) || errors.Is(err, ErrOutputAccess) || errors.Is(err, ErrRecoveryRequired) {
		return filesystemFailure(err)
	}
	return ""
}
