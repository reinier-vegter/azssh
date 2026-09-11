package inventory

import (
	"reflect"
	"testing"
)

func TestDisplayTagsExcludesControlCharacters(t *testing.T) {
	tags := DisplayTags(map[string]string{
		"Environment": "production",
		"Owner\n":     "platform",
		"Team":        "core\x1b]0;unsafe\a",
		"Application": "catalog",
		"NotRelevant": "ignored",
	})
	want := map[string]string{"Environment": "production", "Application": "catalog"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("DisplayTags() = %#v, want %#v", tags, want)
	}
}
