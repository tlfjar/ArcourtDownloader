//go:build !windows

package arcourt

import "os"

// State-machine substitute only: consumes the source without replacing a name.
// This does not claim to test the Windows syscall or a FAT filesystem on Unix.
func testMoveNoReplace(root *os.Root, source, target string) error {
	if err := root.Link(source, target); err != nil {
		return err
	}
	return root.Remove(source)
}
