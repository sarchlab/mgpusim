package insts

import (
	"encoding/json"
	"strings"
	"testing"
)

//nolint:musttag // Exercise the existing checkpoint representation without changing its field names.
func TestMetadataRejectsExecutableCheckpoint(t *testing.T) {
	data, err := json.Marshal(CodeObjectSnapshot(&KernelCodeObject{Data: []byte{1, 2, 3}}))
	if err != nil {
		t.Fatal(err)
	}
	var metadata CodeObjectMetadata
	if err = json.Unmarshal(data, &metadata); err == nil || !strings.Contains(err.Error(), "rebuild checkpoint") {
		t.Fatalf("old executable snapshot accepted: %v", err)
	}
}
