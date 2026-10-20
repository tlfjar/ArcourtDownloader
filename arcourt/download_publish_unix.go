//go:build !windows

package arcourt

import (
	"errors"
	"golang.org/x/sys/unix"
)

// Non-Windows filesystems retain the existing hard-link contract.
func (d *downloadDisk) selectPublisher() error { return nil }

func publicationError(err error) error {
	if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) {
		return errors.Join(ErrUnsupportedFilesystem, err)
	}
	return err
}
