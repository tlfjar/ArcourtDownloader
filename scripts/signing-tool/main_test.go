package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func fixture() ([]byte, []byte) {
	u := make([]byte, 513)
	copy(u, "MZ")
	binary.LittleEndian.PutUint32(u[60:], 64)
	copy(u[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(u[68:], 0x8664)
	binary.LittleEndian.PutUint16(u[84:], 240)
	binary.LittleEndian.PutUint16(u[88:], 0x20b)
	binary.LittleEndian.PutUint32(u[196:], 16)
	s := append(bytes.Clone(u), make([]byte, 23)...)
	binary.LittleEndian.PutUint32(s[232:], 520)
	binary.LittleEndian.PutUint32(s[236:], 16)
	binary.LittleEndian.PutUint32(s[520:], 12)
	binary.LittleEndian.PutUint16(s[524:], 0x200)
	binary.LittleEndian.PutUint16(s[526:], 2)
	binary.LittleEndian.PutUint32(s[152:], 1234)
	return u, s
}

func TestAssociation(t *testing.T) {
	u, s := fixture()
	if err := associate(u, s); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"payload":             func(b []byte) []byte { b[400]++; return b },
		"alignment":           func(b []byte) []byte { b[514]++; return b },
		"certificate padding": func(b []byte) []byte { b[535]++; return b },
		"overlay":             func(b []byte) []byte { return append(b, 0) },
		"wrong offset":        func(b []byte) []byte { b[232]++; return b },
		"certificate type":    func(b []byte) []byte { b[526]++; return b },
		"truncated":           func(b []byte) []byte { return b[:80] },
	} {
		t.Run(name, func(t *testing.T) {
			if associate(u, mutate(bytes.Clone(s))) == nil {
				t.Fatal("accepted changed executable")
			}
		})
	}
	if associate(s, s) == nil {
		t.Fatal("accepted already signed input")
	}
}

func FuzzAssociation(f *testing.F) {
	u, s := fixture()
	f.Add(u, s)
	f.Add([]byte{}, []byte{})
	f.Fuzz(func(t *testing.T, u, s []byte) { _ = associate(u, s) })
}
