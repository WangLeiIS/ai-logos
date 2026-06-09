package book

import (
	"reflect"
	"testing"
)

func TestNormalizeTagsTrimsLowercasesDeduplicates(t *testing.T) {
	got, err := NormalizeTags([]string{" 激光焊接 ", "LASER", "laser", "接头强度"})
	if err != nil {
		t.Fatalf("NormalizeTags returned error: %v", err)
	}

	want := []string{"激光焊接", "laser", "接头强度"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeTags() = %#v, want %#v", got, want)
	}
}

func TestNormalizeTagsRejectsEmptyResult(t *testing.T) {
	if _, err := NormalizeTags([]string{" ", "\t"}); err == nil {
		t.Fatal("NormalizeTags accepted tags that normalize to empty")
	}
}

func TestNormalizeTagsRejectsEmptyTagAmongValidTags(t *testing.T) {
	if _, err := NormalizeTags([]string{"laser", " ", "strength"}); err == nil {
		t.Fatal("NormalizeTags silently ignored an empty normalized tag")
	}
}
