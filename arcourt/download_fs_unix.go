//go:build !windows

package arcourt

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func isReparse(info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }

func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrOutputBusy
	}
	return err
}
