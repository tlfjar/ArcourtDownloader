package arcourt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// DownloadService is shared by application frontends. One preview or download
// may run at a time. The caller owns the fetcher's lifetime and cancels through
// context; browser fetchers must also be closed on application shutdown.
type DownloadService struct {
	fetcher    CaseFetcher
	active     atomic.Bool
	diskBefore func(operation, name string) error
}

func NewDownloadService(fetcher CaseFetcher) (*DownloadService, error) {
	if fetcher == nil {
		return nil, errors.New("a case fetcher is required")
	}
	return &DownloadService{fetcher: fetcher}, nil
}

func (s *DownloadService) Preview(ctx context.Context, caseNumber string) (*CasePreview, error) {
	if !s.active.CompareAndSwap(false, true) {
		return nil, ErrDownloadBusy
	}
	defer s.active.Store(false)
	if !looksLikeCaseNumber(caseNumber) {
		return nil, ErrInvalidCaseNumber
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := s.fetcher.PreviewCaseDocuments(ctx, normalizeCaseNumber(caseNumber))
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrCaseNumberMismatch
	}
	if err := validateCaseNumberMatch(p.CaseInfo.CaseNumber, caseNumber); err != nil {
		return nil, err
	}
	return p, nil
}

func emitDownload(ch chan<- DownloadEvent, event DownloadEvent) {
	select {
	case ch <- event:
	default:
	}
}

func localRecord(entry DocketEntry) LocalDocumentResult {
	return LocalDocumentResult{DocumentID: DocumentID(entry.SourceURL), Description: entry.DocketDescription, FilingDate: entry.FilingDate, Timestamp: time.Now().UTC()}
}

// Persist only controlled messages. A custom fetcher can return errors containing
// credentials, request URLs, response bodies, or other sensitive transport data.
func localFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrManifest):
		return "manifest could not be saved; a published PDF may require reconciliation"
	case errors.Is(err, context.Canceled):
		return "download canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "download deadline exceeded"
	case errors.Is(err, ErrDocumentUnavailable):
		return "selected document unavailable"
	case errors.Is(err, ErrDocumentLimit):
		return "document skipped by fetch limit"
	case errors.Is(err, ErrInvalidPDF):
		return "invalid or incomplete PDF"
	case errors.Is(err, ErrUnsafeDestination):
		return "unsafe output destination"
	default:
		return "document download or local persistence failed"
	}
}

func setLocalError(r *LocalDocumentResult, err error) {
	r.Status = DocumentFailed
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		r.Status = DocumentCanceled
	}
	r.Err, r.Error = err, localFailure(err)
}

func summarizeLocal(result *LocalDownloadResult) {
	c := DownloadCounts{Selected: len(result.Documents)}
	saved := 0
	limitSkipped := false
	for _, r := range result.Documents {
		if r.Saved {
			saved++
		}
		switch r.Status {
		case DocumentSucceeded:
			c.Succeeded++
		case DocumentUnavailable:
			c.Unavailable++
		case DocumentSkipped:
			c.Skipped++
			limitSkipped = limitSkipped || r.SkipReason == SkipLimit
		case DocumentCanceled:
			c.Canceled++
		default:
			c.Failed++
		}
	}
	result.Counts = c
	result.Partial = saved > 0 && (c.Failed+c.Unavailable+c.Canceled > 0 || limitSkipped)
}

// Download saves exactly the distinct, nonblank selected identities. Empty
// selection creates nothing. The result always partitions the selected set,
// including setup errors and cancellation; inspect it even if err is non-nil.
func (s *DownloadService) Download(ctx context.Context, req DownloadRequest, events chan<- DownloadEvent) (result *LocalDownloadResult, err error) {
	result = &LocalDownloadResult{CaseNumber: normalizeCaseNumber(req.CaseNumber)}
	entries := map[string]DocketEntry{}
	var selected []string
	for _, entry := range req.Selection {
		id := normalizeSourceURL(entry.SourceURL)
		if id == "" {
			continue
		}
		if _, ok := entries[id]; ok {
			continue
		}
		entry.SourceURL = id
		entries[id] = entry
		selected = append(selected, id)
		result.Documents = append(result.Documents, localRecord(entry))
	}
	defer func() {
		summarizeLocal(result)
		if err != nil {
			for _, r := range result.Documents {
				if r.Saved {
					result.Partial = true
					break
				}
			}
		}
		for _, r := range result.Documents {
			if r.Status == DocumentFailed || r.Status == DocumentUnavailable || r.Status == DocumentCanceled || r.SkipReason == SkipLimit {
				err = errors.Join(err, ErrDocumentsIncomplete)
				break
			}
		}
		emitDownload(events, DownloadEvent{Kind: DownloadFinished, Counts: result.Counts})
	}()
	failAll := func(cause error) (*LocalDownloadResult, error) {
		for i := range result.Documents {
			if result.Documents[i].Status == "" {
				setLocalError(&result.Documents[i], cause)
			}
		}
		return result, cause
	}
	if !s.active.CompareAndSwap(false, true) {
		return failAll(ErrDownloadBusy)
	}
	defer s.active.Store(false)
	if !looksLikeCaseNumber(req.CaseNumber) {
		return failAll(ErrInvalidCaseNumber)
	}
	skipped := map[string]bool{}
	for _, id := range normalizedStringSlice(req.SkipSourceURLs) {
		if _, ok := entries[id]; !ok {
			return failAll(errors.New("explicit skips must belong to the selection"))
		}
		skipped[id] = true
	}
	if len(selected) == 0 {
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return failAll(err)
	}
	output, err := openOutputDirectory(req.OutputDirectory)
	if err != nil {
		return failAll(err)
	}
	defer output.Close()
	caseDir := "case-" + strings.ToLower(result.CaseNumber)
	names, err := (&downloadDisk{root: output}).names()
	if err != nil {
		return failAll(err)
	}
	for _, name := range names {
		if strings.EqualFold(name, caseDir) && name != caseDir {
			return failAll(ErrUnsafeDestination)
		}
	}
	if err := output.Mkdir(caseDir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return failAll(err)
	}
	root, err := openDirectory(output, caseDir)
	if err != nil {
		return failAll(err)
	}
	defer root.Close()
	lock, err := lockOutput(root)
	if err != nil {
		return failAll(err)
	}
	defer lock.Close()
	result.Directory = filepath.Join(req.OutputDirectory, caseDir)
	result.ManifestPath = filepath.Join(result.Directory, ManifestFilename)
	store := &downloadStore{disk: downloadDisk{root: root, before: s.diskBefore}}
	if err := store.load(result.CaseNumber); err != nil {
		return failAll(err)
	}
	if err := store.recover(ctx); err != nil {
		return failAll(err)
	}
	// Establish a valid manifest before network I/O and detect setup write errors.
	if err := store.save(); err != nil {
		return failAll(err)
	}
	sink := &localSink{store: store, selected: map[string]DocketEntry{}, results: map[string]LocalDocumentResult{}, events: events}
	defer func() { err = errors.Join(err, sink.cleanup()) }()
	var remaining []string
	for i, id := range selected {
		r := &result.Documents[i]
		switch {
		case ctx.Err() != nil:
			setLocalError(r, ctx.Err())
		case skipped[id]:
			r.Status, r.SkipReason = DocumentSkipped, SkipExplicit
		default:
			existing, ok, verifyErr := store.verified(ctx, r.DocumentID)
			if verifyErr != nil {
				setLocalError(r, verifyErr)
			} else if ok {
				*r = existing
				r.Status, r.SkipReason, r.Err, r.Error = DocumentSkipped, SkipVerified, nil, ""
				r.Timestamp = time.Now().UTC()
			} else {
				remaining = append(remaining, id)
				sink.selected[id] = entries[id]
			}
		}
	}
	var fetched *DownloadResult
	if len(remaining) > 0 && ctx.Err() == nil {
		fetched, err = s.fetcher.DownloadCaseDocuments(ctx, result.CaseNumber, DocumentSelection{SourceURLs: remaining}, sink)
	}
	byID := map[string]DocumentOutcome{}
	if fetched != nil {
		for _, out := range fetched.Outcomes {
			byID[normalizeSourceURL(out.Document.SourceURL)] = out
		}
	}
	for i, id := range selected {
		r := &result.Documents[i]
		if r.Status == "" {
			if committed, ok := sink.results[id]; ok {
				*r = committed
			} else if out, ok := byID[id]; ok {
				if out.Document.DocketDescription != "" {
					r.Description = out.Document.DocketDescription
				}
				if out.Document.FilingDate != "" {
					r.FilingDate = out.Document.FilingDate
				}
				r.Status, r.Err, r.Error = out.Status, out.Err, localFailure(out.Err)
				switch out.Status {
				case DocumentSkippedLimit:
					r.Status, r.SkipReason = DocumentSkipped, SkipLimit
				case DocumentFailed, DocumentUnavailable, DocumentCanceled:
				default:
					setLocalError(r, errors.New("fetcher returned no committed PDF"))
				}
			} else if ctx.Err() != nil {
				setLocalError(r, ctx.Err())
			} else {
				cause := err
				if cause == nil {
					cause = errors.New("fetcher omitted a selected document")
				}
				setLocalError(r, cause)
			}
		}
		store.put(*r)
		err = errors.Join(err, r.Err)
		emitDownload(events, DownloadEvent{Kind: DownloadDocumentDone, DocumentID: r.DocumentID, Document: *r})
	}
	// Persist failed/unavailable/skipped/canceled outcomes too. Successful commits
	// were already recorded individually so a later failure cannot erase them.
	if saveErr := store.save(); saveErr != nil {
		err = errors.Join(err, saveErr)
	}
	if ctx.Err() != nil {
		err = errors.Join(err, ctx.Err())
	}
	return result, err
}
