// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

// Logical reference storage includes exact retained string/payload bytes plus
// conservative fixed map/entry accounting. It is an admission unit, not a claim
// about the Go heap or process RSS. Response bodies have separate request limits.
const defaultReferenceLimit int64 = 32 << 20
const referenceMapOverhead int64 = 64
const referenceEntryOverhead int64 = 256

func referenceLimitError() error {
	return failure("RESOURCE_LIMIT", "工作階段的記憶體容量已達上限；已保留先前資料，請縮小取得範圍或登出後重新取得。")
}

// SetReferenceBudget attaches an aggregate admission callback to a fresh
// connector. The callback runs under the connector mutex; it must never acquire
// an application/session/connector lock, and must always accept a release.
// Replacing a callback or attaching after authentication/reference creation is
// rejected so retained bytes cannot escape their original accounting owner.
func (s *Session) SetReferenceBudget(budget func(previous, next int64) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if budget == nil || s.closed || s.referenceBudget != nil || s.token != "" || s.bootstrapped || s.challengeID != "" || s.hasReferences() {
		return failure("INVALID_INPUT", "記憶體容量控管只能在新工作階段建立時設定。")
	}
	s.referenceBudget = budget
	s.referenceGroups = make(map[string]int64)
	return nil
}

func (s *Session) hasReferences() bool {
	return len(s.selfPaidReferences)+len(s.westernReferences)+len(s.medicalReferences)+len(s.providerReferences)+len(s.imagingReferences)+len(s.officialFileReferences)+len(s.documentReferences) > 0
}

func referenceStrings(values ...string) int64 {
	var size int64
	for _, value := range values {
		size += int64(len(value))
	}
	return size
}

func referenceMapSize[T any](refs map[string]T, size func(T) int64) int64 {
	if len(refs) == 0 {
		return 0
	}
	total := referenceMapOverhead
	for key, ref := range refs {
		total += referenceEntryOverhead + int64(len(key)) + size(ref)
	}
	return total
}

func selfPaidReferenceSize(ref string) int64 { return int64(len(ref)) }
func westernReferenceSize(ref westernReference) int64 {
	return referenceStrings(ref.rowID, ref.sourceType, ref.date, ref.institution)
}
func medicalReferenceSize(ref medicalReference) int64 {
	return referenceStrings(ref.category, ref.rowID, ref.sourceType, ref.date, ref.endDate, ref.institution)
}
func providerReferenceSize(ref providerReference) int64 {
	return referenceStrings(ref.rowID, ref.sourceType, ref.date, ref.dateRole, ref.institution, ref.title, ref.unit, ref.kind, ref.originKey, ref.parentReference, ref.query.StartDate, ref.query.EndDate)
}
func officialFileReferenceSize(ref officialFileReference) int64 {
	return referenceStrings(ref.sequence, ref.format, ref.date)
}
func documentReferenceSize(ref documentReference) int64 {
	return referenceStrings(ref.category, ref.parent, ref.kind, ref.rowID, ref.sourceType, ref.sequence, ref.format, ref.date, ref.downloadID) + int64(cap(ref.payload))
}

// Initialization also supports focused connector fixtures that seed maps
// directly. Production creates references only through the admission helpers.
// Ordinary detail reads never rescan unrelated category reference maps.
func (s *Session) initializeReferenceAccounting() {
	if s.referenceGroups != nil {
		return
	}
	s.referenceGroups = map[string]int64{
		"self_paid":      referenceMapSize(s.selfPaidReferences, selfPaidReferenceSize),
		"western":        referenceMapSize(s.westernReferences, westernReferenceSize),
		"imaging":        referenceMapSize(s.imagingReferences, westernReferenceSize),
		"official_files": referenceMapSize(s.officialFileReferences, officialFileReferenceSize),
		"documents":      referenceMapSize(s.documentReferences, documentReferenceSize),
	}
	for category, refs := range s.medicalReferences {
		s.referenceGroups["medical/"+category] = referenceMapSize(refs, medicalReferenceSize)
	}
	for category, refs := range s.providerReferences {
		s.referenceGroups["provider/"+category] = referenceMapSize(refs, providerReferenceSize)
	}
	for _, size := range s.referenceGroups {
		s.referenceBytes += size
	}
}

func (s *Session) reserveReferenceGroups(changes map[string]int64) error {
	s.initializeReferenceAccounting()
	next := s.referenceBytes
	for group, size := range changes {
		next += size - s.referenceGroups[group]
	}
	if next > s.referenceBytes {
		if next > defaultReferenceLimit || (s.referenceBudget != nil && !s.referenceBudget(s.referenceBytes, next)) {
			return referenceLimitError()
		}
	} else if next < s.referenceBytes && s.referenceBudget != nil {
		// Releases cannot be denied or leave retained accounting after expiry.
		s.referenceBudget(s.referenceBytes, next)
	}
	s.referenceBytes = next
	for group, size := range changes {
		if size == 0 {
			delete(s.referenceGroups, group)
		} else {
			s.referenceGroups[group] = size
		}
	}
	return nil
}

func (s *Session) releaseReferenceBudget() {
	if s.referenceBudget != nil && s.referenceBytes > 0 {
		s.referenceBudget(s.referenceBytes, 0)
	}
	s.referenceBytes = 0
	s.referenceGroups = nil
}

func (s *Session) remainingDocumentReferences(category, parent string, entireCategory bool, additions map[string]documentReference) (int, int64) {
	count := len(additions)
	size := referenceMapSize(additions, documentReferenceSize)
	if count > 0 {
		size -= referenceMapOverhead
	}
	for key, ref := range s.documentReferences {
		if ref.category == category && (entireCategory || ref.parent == parent) {
			continue
		}
		count++
		size += referenceEntryOverhead + int64(len(key)) + documentReferenceSize(ref)
	}
	if count > 0 {
		size += referenceMapOverhead
	}
	return count, size
}

// A list refresh reserves its replacement and its retired document handles as
// one change. Rejection leaves both the previous list and documents usable.
func (s *Session) reserveCategoryReferences(group string, size int64, category string) error {
	_, documents := s.remainingDocumentReferences(category, "", true, nil)
	if err := s.reserveReferenceGroups(map[string]int64{group: size, "documents": documents}); err != nil {
		return err
	}
	s.clearDocumentCategory(category)
	return nil
}

func (s *Session) consumeDocumentReference(reference string) {
	ref, ok := s.documentReferences[reference]
	if !ok {
		return
	}
	s.initializeReferenceAccounting()
	next := s.referenceGroups["documents"] - referenceEntryOverhead - int64(len(reference)) - documentReferenceSize(ref)
	if len(s.documentReferences) == 1 {
		next = 0
	}
	// This is always a release, so reservation cannot fail.
	_ = s.reserveReferenceGroups(map[string]int64{"documents": next})
	delete(s.documentReferences, reference)
}
