package image

import "testing"

func TestReqFresh(t *testing.T) {
	r := Req{ArticleID: 42, Gen: 7}

	if !r.Fresh(42, 7) {
		t.Fatal("matching id+gen should be fresh")
	}
	if r.Fresh(42, 8) {
		t.Fatal("stale generation must not be fresh")
	}
	if r.Fresh(43, 7) {
		t.Fatal("different article must not be fresh")
	}
	if r.Fresh(0, 0) {
		t.Fatal("zero current state must not be fresh")
	}
}
