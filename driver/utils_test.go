package driver

import (
	"reflect"
	"testing"
)

func Test_cleanISCSIName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "test-1234", want: "test1234"},
		{input: "teST.1234", want: "test1234"},
		{input: "test1234", want: "test1234"},
	}

	for _, table := range tests {
		got := cleanISCSIName(table.input)
		if !reflect.DeepEqual(table.want, got) {
			t.Fatalf("expected: %v, got: %v", table.want, got)
		}
	}
}
