package arcourt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

const maxManifestBytes = 8 << 20

var receiptPattern = regexp.MustCompile(`^\.arcourt-receipt-[a-f0-9]{32}\.json$`)
var pdfTempPattern = regexp.MustCompile(`^\.arcourt-pdf-[a-f0-9]{32}\.tmp$`)

type downloadStore struct {
	disk         downloadDisk
	manifest     DownloadManifest
	manifestInfo os.FileInfo
}

type publishReceipt struct {
	Version    int                 `json:"version"`
	CaseNumber string              `json:"case_number"`
	Temporary  string              `json:"temporary"`
	Document   LocalDocumentResult `json:"document"`
}

func readLocalJSON(root *os.Root, name string, value any) (os.FileInfo, error) {
	f, err := openRegular(root, name, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxManifestBytes {
		return nil, ErrManifest
	}
	data, err := io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxManifestBytes {
		return nil, ErrManifest
	}
	if err = json.Unmarshal(data, value); err != nil {
		return nil, ErrManifest
	}
	return info, nil
}

func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}

func validRecord(r LocalDocumentResult) bool {
	if !validDigest(r.DocumentID) || r.Timestamp.IsZero() {
		return false
	}
	switch r.Status {
	case DocumentSucceeded, DocumentFailed, DocumentUnavailable, DocumentSkipped, DocumentCanceled:
	default:
		return false
	}
	if r.Filename != "" && (!safeComponent(r.Filename) || !strings.HasSuffix(r.Filename, ".pdf")) {
		return false
	}
	if r.Saved {
		return r.Filename != "" && r.Size >= 8 && validDigest(r.SHA256)
	}
	return r.Status != DocumentSucceeded && r.Filename == "" && r.Size == 0 && r.SHA256 == ""
}

func (s *downloadStore) load(caseNumber string) error {
	s.manifest = DownloadManifest{Version: ManifestVersion, CaseNumber: caseNumber, Documents: []LocalDocumentResult{}}
	// A differently cased reserved manifest name is also a collision on Unix.
	names, err := s.disk.names()
	if err != nil {
		return err
	}
	for _, name := range names {
		if strings.EqualFold(name, ManifestFilename) && name != ManifestFilename {
			return ErrManifest
		}
	}
	var m DownloadManifest
	info, err := readLocalJSON(s.disk.root, ManifestFilename, &m)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.Join(ErrManifest, err)
	}
	if m.Version != ManifestVersion || m.CaseNumber != caseNumber {
		return ErrManifest
	}
	seen := map[string]bool{}
	for _, r := range m.Documents {
		if !validRecord(r) {
			return ErrManifest
		}
		if r.Filename != "" {
			key := strings.ToLower(r.Filename)
			if seen[key] {
				return ErrManifest
			}
			seen[key] = true
		}
	}
	s.manifest, s.manifestInfo = m, info
	return nil
}

func (s *downloadStore) put(record LocalDocumentResult) {
	record.Err = nil
	for i, r := range s.manifest.Documents {
		if r.DocumentID == record.DocumentID && r.Filename == record.Filename {
			s.manifest.Documents[i] = record
			return
		}
	}
	s.manifest.Documents = append(s.manifest.Documents, record)
}

func (s *downloadStore) save() (err error) {
	s.manifest.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(s.manifest, "", "  ")
	if err != nil || len(data)+1 > maxManifestBytes {
		return ErrManifest
	}
	tmp, err := s.disk.writeClosed("manifest", append(data, '\n'))
	if err != nil {
		return errors.Join(ErrManifest, err)
	}
	defer func() {
		err = errors.Join(err, s.disk.remove(tmp))
		if err != nil {
			err = errors.Join(ErrManifest, err)
		}
	}()
	current, err := s.disk.root.Lstat(ManifestFilename)
	if s.manifestInfo == nil {
		if !errors.Is(err, os.ErrNotExist) {
			return ErrManifest
		}
		// The first manifest must not clobber an independently created file.
		if err = s.disk.check("manifest.publish", ManifestFilename); err == nil {
			err = s.disk.root.Link(tmp, ManifestFilename)
		}
	} else {
		if err != nil || isReparse(current) || !os.SameFile(current, s.manifestInfo) {
			return ErrManifest
		}
		if err = s.disk.check("manifest.rename", ManifestFilename); err == nil {
			err = s.disk.root.Rename(tmp, ManifestFilename)
		}
	}
	if err != nil {
		return errors.Join(ErrManifest, err)
	}
	s.manifestInfo, err = s.disk.root.Lstat(ManifestFilename)
	return err
}

// inspectPDF reopens a closed file and verifies its actual bytes. It is bounded
// in memory and checks cancellation between reads, independent of fetcher claims.
func (s *downloadStore) inspectPDF(ctx context.Context, name string) (int64, string, error) {
	f, err := openRegular(s.disk.root, name, os.O_RDONLY)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, "", err
	}
	if info.Size() > 1<<30 {
		return 0, "", ErrDocumentTooLarge
	}
	h := sha256.New()
	buf := make([]byte, 32<<10)
	var header, tail []byte
	var size int64
	reader := contextReader{ctx: ctx, reader: f}
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			size += int64(n)
			if size > 1<<30 {
				return 0, "", ErrDocumentTooLarge
			}
			if len(header) < 8 {
				header = append(header, chunk[:min(8-len(header), len(chunk))]...)
			}
			h.Write(chunk)
			if n >= 1024 {
				tail = append(tail[:0], chunk[n-1024:]...)
			} else {
				drop := max(0, len(tail)+n-1024)
				tail = append(tail[:copy(tail, tail[drop:])], chunk...)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, "", readErr
		}
	}
	if len(header) < 8 || !bytes.HasPrefix(header, []byte("%PDF-")) || (header[5] != '1' && header[5] != '2') || header[6] != '.' || header[7] < '0' || header[7] > '9' || !bytes.HasSuffix(bytes.TrimSpace(tail), []byte("%%EOF")) {
		return 0, "", ErrInvalidPDF
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

func (s *downloadStore) verified(ctx context.Context, id string) (LocalDocumentResult, bool, error) {
	for i := len(s.manifest.Documents) - 1; i >= 0; i-- {
		r := s.manifest.Documents[i]
		if r.DocumentID != id || !r.Saved {
			continue
		}
		size, hash, err := s.inspectPDF(ctx, r.Filename)
		if ctx.Err() != nil {
			return LocalDocumentResult{}, false, ctx.Err()
		}
		if err == nil && size == r.Size && hash == r.SHA256 {
			return r, true, nil
		}
	}
	return LocalDocumentResult{}, false, nil
}

func (s *downloadStore) receipt(temp string, r LocalDocumentResult) (name string, err error) {
	data, err := json.Marshal(publishReceipt{ManifestVersion, s.manifest.CaseNumber, temp, r})
	if err != nil || len(data) > maxManifestBytes {
		return "", ErrManifest
	}
	staged, err := s.disk.writeClosed("receipt", data)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, s.disk.remove(staged)) }()
	name = ".arcourt-receipt-" + strings.TrimSuffix(strings.TrimPrefix(temp, ".arcourt-pdf-"), ".tmp") + ".json"
	if err = s.disk.check("receipt.publish", name); err == nil {
		err = s.disk.root.Link(staged, name)
	}
	if err != nil {
		return "", err
	}
	return name, nil
}

func (s *downloadStore) recover(ctx context.Context) error {
	names, err := s.disk.names()
	if err != nil {
		return err
	}
	for _, name := range names {
		if !receiptPattern.MatchString(name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var receipt publishReceipt
		if _, err := readLocalJSON(s.disk.root, name, &receipt); err != nil {
			return errors.Join(ErrManifest, err)
		}
		r := receipt.Document
		if receipt.Version != ManifestVersion || receipt.CaseNumber != s.manifest.CaseNumber || !pdfTempPattern.MatchString(receipt.Temporary) || !validRecord(r) || !r.Saved {
			return ErrManifest
		}
		// The complete temporary hard link witnesses our ownership of the final
		// file. Mere matching bytes or a predictable name do not establish it.
		tmp, tmpErr := s.disk.root.Lstat(receipt.Temporary)
		final, finalErr := s.disk.root.Lstat(r.Filename)
		if tmpErr != nil || finalErr != nil || isReparse(tmp) || isReparse(final) || !os.SameFile(tmp, final) {
			continue
		}
		size, hash, err := s.inspectPDF(ctx, r.Filename)
		if err != nil || size != r.Size || hash != r.SHA256 {
			continue
		}
		s.put(r)
		if err := s.save(); err != nil {
			return err
		}
		if err := s.disk.remove(name); err != nil {
			return err
		}
		if err := s.disk.remove(receipt.Temporary); err != nil {
			return err
		}
	}
	return nil
}
