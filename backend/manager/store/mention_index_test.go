package store

import "testing"

func TestInvalidateGlobalMentionIndex(t *testing.T) {
	s := &Store{}
	s.globalMentionIndex = &GlobalMentionIndex{}
	s.globalMentionIndexes = map[string]*GlobalMentionIndex{"org-a": &GlobalMentionIndex{}}
	s.InvalidateGlobalMentionIndex()
	if s.globalMentionIndex != nil {
		t.Fatal("InvalidateGlobalMentionIndex should clear the cached index")
	}
	if s.globalMentionIndexes != nil {
		t.Fatal("InvalidateGlobalMentionIndex should clear tenant-scoped indexes")
	}
}
