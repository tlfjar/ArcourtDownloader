//go:build windows

package arcourt

import (
	"os"
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
