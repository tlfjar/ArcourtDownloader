// signing-tool compares a provider's output to an immutable unsigned PE input.
// Windows Authenticode trust and signer policy are checked separately in PowerShell.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

func peOffsets(b []byte) (int, int, error) {
	if len(b) < 64 || string(b[:2]) != "MZ" {
		return 0, 0, fmt.Errorf("invalid DOS header")
	}
	p := uint64(binary.LittleEndian.Uint32(b[60:64]))
	// Require the PE32+ Windows amd64 format produced by our build scripts.
	if p+24+152 > uint64(len(b)) || string(b[p:p+4]) != "PE\x00\x00" || binary.LittleEndian.Uint16(b[p+4:]) != 0x8664 || binary.LittleEndian.Uint16(b[p+20:]) < 152 || binary.LittleEndian.Uint16(b[p+24:]) != 0x20b || binary.LittleEndian.Uint32(b[p+24+108:]) < 5 {
		return 0, 0, fmt.Errorf("unsupported PE headers")
	}
	return int(p) + 24 + 64, int(p) + 24 + 112 + 4*8, nil
}

func associate(unsigned, signed []byte) error {
	checksum, security, err := peOffsets(unsigned)
	if err != nil {
		return err
	}
	sc, ss, err := peOffsets(signed)
	if err != nil {
		return err
	}
	if sc != checksum || ss != security || !bytes.Equal(unsigned[security:security+8], make([]byte, 8)) {
		return fmt.Errorf("input already signed or PE layout changed")
	}
	offset := uint64(binary.LittleEndian.Uint32(signed[security:]))
	size := uint64(binary.LittleEndian.Uint32(signed[security+4:]))
	expected := (uint64(len(unsigned)) + 7) &^ 7
	if offset != expected || size < 8 || size%8 != 0 || offset+size != uint64(len(signed)) {
		return fmt.Errorf("certificate must be the only appended data")
	}
	cert := signed[offset:]
	length := uint64(binary.LittleEndian.Uint32(cert))
	if length <= 8 || (length+7)&^7 != size || binary.LittleEndian.Uint16(cert[4:]) != 0x200 || binary.LittleEndian.Uint16(cert[6:]) != 2 {
		return fmt.Errorf("invalid WIN_CERTIFICATE")
	}
	for _, b := range signed[len(unsigned):offset] {
		if b != 0 {
			return fmt.Errorf("nonzero alignment padding")
		}
	}
	for _, b := range cert[length:] {
		if b != 0 {
			return fmt.Errorf("nonzero certificate padding")
		}
	}
	u := bytes.Clone(unsigned)
	s := bytes.Clone(signed[:len(unsigned)])
	clear(u[checksum : checksum+4])
	clear(s[checksum : checksum+4])
	clear(s[security : security+8])
	if !bytes.Equal(u, s) {
		return fmt.Errorf("signed executable differs from exact unsigned build input")
	}
	return nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: signing-tool UNSIGNED SIGNED")
		os.Exit(1)
	}
	u, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	s, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	if err := associate(u, s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Exact unsigned/signed PE association verified")
}
