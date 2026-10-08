package arcourt

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

type localSink struct {
	store    *downloadStore
	naming   *namingSession
	selected map[string]DocketEntry
	results  map[string]LocalDocumentResult
	writers  []*localWriter
	events   chan<- DownloadEvent
}

func (s *localSink) OpenDocument(ctx context.Context, entry DocketEntry) (DocumentWriter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := normalizeSourceURL(entry.SourceURL)
	if _, ok := s.selected[id]; !ok {
		return nil, errors.New("fetcher opened an unselected document")
	}
	if _, ok := s.results[id]; ok {
		return nil, errors.New("document already published")
	}
	for _, w := range s.writers {
		if !w.closed {
			return nil, errors.New("parallel document writers are not supported")
		}
	}
	f, name, err := s.store.disk.temp("pdf")
	if err != nil {
		return nil, err
	}
	w := &localWriter{sink: s, ctx: ctx, file: f, temp: name, source: id, record: localRecord(entry)}
	s.writers = append(s.writers, w)
	emitDownload(s.events, DownloadEvent{Kind: DownloadStarted, DocumentID: w.record.DocumentID})
	return w, nil
}

func (s *localSink) cleanup() error {
	var err error
	for _, w := range s.writers {
		err = errors.Join(err, w.Abort())
	}
	return err
}

type localWriter struct {
	sink                  *localSink
	ctx                   context.Context
	file                  *os.File
	temp, source, receipt string
	record                LocalDocumentResult
	closed, published     bool
	bytes                 int64
}

func (w *localWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, os.ErrClosed
	}
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if w.bytes+int64(len(p)) > 1<<30 {
		return 0, ErrDocumentTooLarge
	}
	if err := w.sink.store.disk.check("pdf.write", w.temp); err != nil {
		return 0, err
	}
	n, err := w.file.Write(p)
	w.bytes += int64(n)
	emitDownload(w.sink.events, DownloadEvent{Kind: DownloadTransferring, DocumentID: w.record.DocumentID, Bytes: w.bytes})
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (w *localWriter) Commit() (err error) {
	if w.closed {
		return os.ErrClosed
	}
	s := w.sink.store
	defer func() {
		if w.published {
			if err != nil {
				setLocalError(&w.record, err)
			}
			w.sink.results[w.source] = w.record
		}
	}()
	err = w.ctx.Err()
	if err == nil {
		err = s.disk.check("pdf.sync", w.temp)
	}
	if err == nil {
		err = w.file.Sync()
	}
	err = errors.Join(err, s.disk.check("pdf.close", w.temp), w.file.Close())
	w.closed = true
	if err != nil {
		return err
	}
	size, hash, err := s.inspectPDF(w.ctx, w.temp)
	if err != nil {
		return err
	}
	w.record.Size, w.record.SHA256 = size, hash
	w.record.Status, w.record.Saved = DocumentSucceeded, true
	labelSource := w.record.Description
	if w.sink.naming != nil {
		f, openErr := openRegular(s.disk.root, w.temp, os.O_RDONLY)
		if openErr != nil {
			w.record.Naming = &NamingOutcome{Source: "deterministic", Reason: "unreadable", Strategy: namingStrategyVersion}
		} else {
			w.record.Naming = w.sink.naming.name(w.ctx, f, size)
			_ = f.Close()
		}
		if w.record.Naming.Source == "ai" {
			labelSource = w.record.Naming.Label
		}
	}
	label := strings.TrimSuffix(SanitizeFilename(w.record.FilingDate+" "+labelSource, "document"), ".pdf")
	// Sanitization expands some characters (notably '&' -> 'and'). Check the
	// actual stem as well as the raw text before accepting an AI label so the
	// filename maker never truncates away a material qualifier or exceeds its
	// basename budget after identity and a collision suffix are appended.
	if w.record.Naming != nil && w.record.Naming.Source == "ai" &&
		(len(strings.TrimSpace(w.record.FilingDate+" "+labelSource)) > 90 || len(label) > 90) {
		w.record.Naming.Source, w.record.Naming.Label, w.record.Naming.Reason = "deterministic", "", "length_exhaustion"
		labelSource = w.record.Description
		label = strings.TrimSuffix(SanitizeFilename(w.record.FilingDate+" "+labelSource, "document"), ".pdf")
	}
	// Leave room for identity, collision suffixes, and the extension.
	if w.record.Naming == nil || w.record.Naming.Source != "ai" {
		if len(label) > 72 {
			label = strings.TrimRight(label[:72], "._-")
		}
	}
	base := label + "-" + w.record.DocumentID[:16] + ".pdf"
	for attempt := 0; attempt < 32; attempt++ {
		name, err := s.disk.distinctName(base, s.manifest.Documents)
		if err != nil {
			return err
		}
		w.record.Filename = name
		w.receipt, err = s.receipt(w.temp, w.record)
		if err != nil {
			return err
		}
		// Once a staged PDF has passed independent validation, an optional naming
		// cancellation uses the deterministic name and bounded local finalization.
		// The download result still reports the canceled job context.
		if w.sink.naming == nil {
			if err = w.ctx.Err(); err != nil {
				return err
			}
		}
		if err = s.disk.check("publish.link", name); err == nil {
			err = s.disk.root.Link(w.temp, name)
		}
		if errors.Is(err, os.ErrExist) {
			if err = s.disk.remove(w.receipt); err != nil {
				return err
			}
			w.receipt = ""
			continue
		}
		if err != nil {
			return err
		}
		w.published = true
		// Keep the receipt AND complete temp hard link until the manifest is safe.
		// Abort after a failed commit must preserve this recovery evidence.
		s.put(w.record)
		if err = s.save(); err != nil {
			return err
		}
		if err = s.disk.remove(w.receipt); err != nil {
			return err
		}
		w.receipt = ""
		return s.disk.remove(w.temp)
	}
	return errors.New("concurrent filename collisions prevented publication")
}

func (w *localWriter) Abort() error {
	var err error
	if !w.closed {
		err = w.file.Close()
		w.closed = true
	}
	if w.published {
		return err
	}
	if removeErr := w.sink.store.disk.remove(w.receipt); removeErr != nil {
		err = errors.Join(err, removeErr)
	} else {
		w.receipt = ""
	}
	if removeErr := w.sink.store.disk.remove(w.temp); removeErr != nil {
		err = errors.Join(err, removeErr)
	} else {
		w.temp = ""
	}
	return err
}
