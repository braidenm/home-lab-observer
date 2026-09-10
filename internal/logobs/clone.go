package logobs

import "time"

func (s Status) Clone() Status {
	out := s
	out.ObservedAt = cloneTime(s.ObservedAt)
	out.AttemptedAt = cloneTime(s.AttemptedAt)
	out.CoverageThrough = cloneTime(s.CoverageThrough)
	out.ReasonCode = cloneReason(s.ReasonCode)
	return out
}

func (c Checkpoint) Clone() Checkpoint {
	out := c
	out.Opaque = cloneBytes(c.Opaque)
	out.PreviousAttemptAt = cloneTime(c.PreviousAttemptAt)
	out.CoverageThrough = cloneTime(c.CoverageThrough)
	return out
}

func (r ReadRequest) Clone() ReadRequest {
	out := r
	out.Checkpoint = r.Checkpoint.Clone()
	return out
}

func (b Batch) Clone() Batch {
	out := b
	out.ReasonCode = cloneReason(b.ReasonCode)
	out.Events = append([]Event(nil), b.Events...)
	out.Discards = append([]DiscardCount(nil), b.Discards...)
	out.NextOpaque = cloneBytes(b.NextOpaque)
	return out
}

func (s Summary) Clone() Summary {
	out := s
	out.Sources = make([]SourceSummary, len(s.Sources))
	for i := range s.Sources {
		out.Sources[i] = s.Sources[i]
		out.Sources[i].Status = s.Sources[i].Status.Clone()
		out.Sources[i].Counts = cloneCounts(s.Sources[i].Counts)
		out.Sources[i].Buckets = make([]SummaryBucket, len(s.Sources[i].Buckets))
		for j := range s.Sources[i].Buckets {
			out.Sources[i].Buckets[j] = s.Sources[i].Buckets[j]
			out.Sources[i].Buckets[j].ReasonCode = cloneReason(s.Sources[i].Buckets[j].ReasonCode)
			out.Sources[i].Buckets[j].Counts = cloneBucketCounts(s.Sources[i].Buckets[j].Counts)
		}
	}
	return out
}

func (s Snapshot) Clone() Snapshot {
	out := s
	out.Status = s.Status.Clone()
	out.Events = append([]Event(nil), s.Events...)
	return out
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneReason(value *ReasonCode) *ReasonCode {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}

func cloneCounts(value *Counts) *Counts {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBucketCounts(value *BucketCounts) *BucketCounts {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
