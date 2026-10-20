//go:build windows

package arcourt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func testMoveNoReplace(root *os.Root, source, target string) error {
	return windowsMoveNoReplace(root, source, target)
}

func TestWindowsFilesystemFixture(t *testing.T) {
	path := os.Getenv("ARCOURT_TEST_VOLUME_ROOT")
	if path == "" {
		t.Skip("no FAT32/exFAT volume fixture; run scripts/test-windows-filesystems.ps1")
	}
	expected := os.Getenv("ARCOURT_TEST_VOLUME_FS")
	if expected != "FAT32" && expected != "exFAT" {
		t.Fatal("fixture requires an explicit FAT32/exFAT expectation")
	}
	root, err := openOutputDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	var name [256]uint16
	if err := windows.GetVolumeInformationByHandle(windows.Handle(dir.Fd()), nil, 0, nil, nil, nil, &name[0], uint32(len(name))); err != nil {
		t.Fatal(err)
	}
	actual := windows.UTF16ToString(name[:])
	if !strings.EqualFold(actual, expected) {
		t.Fatalf("fixture is %s, expected %s", actual, expected)
	}
	t.Logf("native Windows volume verified: %s", actual)
}

func TestMoveWindowsBlockedHandle(t *testing.T) {
	s, _, req := moveFixture(t)
	root, err := openOutputDirectory(req.OutputDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	disk := downloadDisk{root: root}
	if err := s.diskSetup(&disk); err != nil {
		t.Fatal(err)
	}
	source, err := disk.writeClosed("probe", []byte("owned"))
	if err != nil {
		t.Fatal(err)
	}
	// os.Root.Open permits delete sharing on modern Go. Use an explicit
	// Windows reader that denies delete sharing to model a blocking editor.
	path, err := windows.UTF16PtrFromString(filepath.Join(req.OutputDirectory, source))
	if err != nil {
		t.Fatal(err)
	}
	held, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = disk.publish(source, "final.pdf")
	windows.CloseHandle(held)
	if !errors.Is(err, ErrOutputAccess) || errors.Is(err, ErrUnsupportedFilesystem) {
		t.Fatalf("wrong sharing diagnosis: %v", err)
	}
	if _, err := root.Stat("final.pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed publication created final")
	}
	if err := disk.publish(source, "final.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestMoveWindowsSymlinkSource(t *testing.T) {
	s, _, req := moveFixture(t)
	root, err := openOutputDirectory(req.OutputDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	disk := downloadDisk{root: root}
	if err := s.diskSetup(&disk); err != nil {
		t.Fatal(err)
	}
	source, err := disk.writeClosed("probe", []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(req.OutputDirectory, source), filepath.Join(req.OutputDirectory, "link.tmp")); err != nil {
		t.Skipf("filesystem/account cannot create symlinks: %v", err)
	}
	if err := disk.publish("link.tmp", "final.pdf"); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatal("symlink source accepted", err)
	}
}
