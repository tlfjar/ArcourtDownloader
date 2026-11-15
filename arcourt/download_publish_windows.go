//go:build windows

package arcourt

import (
	"errors"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func publicationError(err error) error {
	var status windows.NTStatus
	if errors.As(err, &status) {
		err = status.Errno()
	}
	switch {
	case errors.Is(err, windows.ERROR_NOT_SUPPORTED), errors.Is(err, windows.ERROR_INVALID_FUNCTION), errors.Is(err, windows.ERROR_CALL_NOT_IMPLEMENTED):
		return errors.Join(ErrUnsupportedFilesystem, err)
	case errors.Is(err, windows.ERROR_ACCESS_DENIED), errors.Is(err, windows.ERROR_SHARING_VIOLATION), errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return errors.Join(ErrOutputAccess, err)
	}
	return err
}

func (d *downloadDisk) selectPublisher() error {
	dir, err := d.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	h := windows.Handle(dir.Fd())
	// Resolve the pinned handle, not a drive-letter path that could be remapped.
	var path [32768]uint16
	n, err := windows.GetFinalPathNameByHandle(h, &path[0], uint32(len(path)), 0)
	if err != nil {
		return publicationError(err)
	}
	if n >= uint32(len(path)) || strings.HasPrefix(strings.ToUpper(windows.UTF16ToString(path[:])), `\\?\UNC\`) {
		return ErrUnsafeDestination
	}
	var fs [256]uint16
	var flags uint32
	if err = windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, &flags, &fs[0], uint32(len(fs))); err != nil {
		return publicationError(err)
	}
	if flags&windows.FILE_SUPPORTS_HARD_LINKS != 0 {
		return nil
	}
	// Limit the new contract to local Windows FAT32/exFAT. Other non-link
	// filesystems remain unsupported until their behavior has been tested.
	name := windows.UTF16ToString(fs[:])
	if !strings.EqualFold(name, "FAT32") && !strings.EqualFold(name, "exFAT") {
		return ErrUnsupportedFilesystem
	}
	d.move = windowsMoveNoReplace
	return d.probeMove()
}

// windowsMoveNoReplace uses FileRenameInformation, not Go Rename (which replaces)
// or MoveFileEx (path based). Both opens and the target are relative to the same
// pinned directory handle. ReplaceIfExists is zero. There is no copy fallback.
// https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information
func windowsMoveNoReplace(root *os.Root, source, target string) error {
	if !safeComponent(source) || !safeComponent(target) || source == target {
		return ErrUnsafeDestination
	}
	before, err := root.Lstat(source)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || isReparse(before) {
		return ErrUnsafeDestination
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	sourceName, err := windows.NewNTUnicodeString(source)
	if err != nil {
		return err
	}
	attrs := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(dir.Fd()), ObjectName: sourceName,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var h windows.Handle
	err = windows.NtCreateFile(&h, windows.DELETE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		&attrs, &windows.IO_STATUS_BLOCK{}, nil, 0, windows.FILE_SHARE_READ,
		windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return publicationError(err)
	}
	file := os.NewFile(uintptr(h), source)
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if isReparse(after) || !os.SameFile(before, after) {
		return ErrUnsafeDestination
	}
	// Use a fixed, bounded native layout; Go supplies the HANDLE alignment on
	// amd64, arm64 and 386. FileNameLength is bytes, excluding the terminator.
	info := struct {
		ReplaceIfExists uint32
		RootDirectory   windows.Handle
		FileNameLength  uint32
		FileName        [161]uint16
	}{RootDirectory: windows.Handle(dir.Fd())}
	name, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	if len(name) > len(info.FileName) {
		return ErrUnsafeDestination
	}
	copy(info.FileName[:], name)
	info.FileNameLength = uint32((len(name) - 1) * 2)
	err = windows.NtSetInformationFile(h, &windows.IO_STATUS_BLOCK{}, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), windows.FileRenameInformation)
	return publicationError(err)
}
