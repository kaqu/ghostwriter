package filesystem

import (
	"reflect"
	"testing"
)

func TestDefaultFileSystemAdapter_IsValidUTF8(t *testing.T) {
	adapter := NewDefaultFileSystemAdapter()
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"empty string", []byte(""), true},
		{"valid ascii", []byte("hello"), true},
		{"valid utf-8", []byte("hello, 世界"), true},
		{"invalid utf-8 sequence", []byte{0xff, 0xfe, 0xfd}, false},
		{"valid partial utf-8", []byte("abc\xe2\x82\xac"), true}, // Euro sign
		{"invalid continuation byte", []byte{0xe2, 0x82}, false}, // Incomplete Euro sign
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapter.IsValidUTF8(tt.content); got != tt.want {
				t.Errorf("DefaultFileSystemAdapter.IsValidUTF8() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultFileSystemAdapter_NormalizeNewlines(t *testing.T) {
	adapter := NewDefaultFileSystemAdapter()
	tests := []struct {
		name    string
		content []byte
		want    []byte
	}{
		{"empty", []byte(""), []byte("")},
		{"no newlines", []byte("hello world"), []byte("hello world")},
		{"lf only", []byte("hello\nworld"), []byte("hello\nworld")},
		{"crlf", []byte("hello\r\nworld"), []byte("hello\nworld")},
		{"cr only", []byte("hello\rworld"), []byte("hello\nworld")},
		{"mixed newlines", []byte("line1\r\nline2\rline3\nline4"), []byte("line1\nline2\nline3\nline4")},
		{"multiple crlf", []byte("hello\r\n\r\nworld"), []byte("hello\n\nworld")},
		{"trailing crlf", []byte("hello\r\n"), []byte("hello\n")},
		{"trailing cr", []byte("hello\r"), []byte("hello\n")},
		{"leading crlf", []byte("\r\nhello"), []byte("\nhello")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.NormalizeNewlines(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DefaultFileSystemAdapter.NormalizeNewlines() for %s:\n  got: %q (len %d, cap %d, nil: %v)\n want: %q (len %d, cap %d, nil: %v)",
					tt.name,
					string(got), len(got), cap(got), got == nil,
					string(tt.want), len(tt.want), cap(tt.want), tt.want == nil)
			}
		})
	}
}

func TestDefaultFileSystemAdapter_SplitLines(t *testing.T) {
	adapter := NewDefaultFileSystemAdapter()
	tests := []struct {
		name    string
		content []byte
		want    []string
	}{
		{"empty content", []byte(""), []string{}}, // As per implementation: empty content -> empty slice
		{"single line no newline", []byte("hello"), []string{"hello"}},
		{"single line with lf", []byte("hello\n"), []string{"hello"}},
		{"single line with crlf", []byte("hello\r\n"), []string{"hello"}},
		{"single line with cr", []byte("hello\r"), []string{"hello"}},
		{"multiple lines lf", []byte("line1\nline2\nline3"), []string{"line1", "line2", "line3"}},
		{"multiple lines crlf", []byte("line1\r\nline2\r\nline3"), []string{"line1", "line2", "line3"}},
		{"multiple lines cr", []byte("line1\rline2\rline3"), []string{"line1", "line2", "line3"}},
		{"mixed newlines with trailing lf", []byte("line1\r\nline2\rline3\n"), []string{"line1", "line2", "line3"}},
		{"content with empty lines", []byte("line1\n\nline3\n"), []string{"line1", "", "line3"}},
		{"content starting with newline", []byte("\nline1\nline2"), []string{"", "line1", "line2"}},
		{"content ending with multiple newlines", []byte("line1\n\n"), []string{"line1", ""}}, // "line1\n\n" -> normalized "line1\n\n" -> split ["line1", "", ""] -> trailing "" removed -> ["line1", ""]
		{"only a newline", []byte("\n"), []string{""}},                                        // "\n" -> normalized "\n" -> split ["", ""] -> special case rule -> [""]
		{"only crlf", []byte("\r\n"), []string{""}},
		{"only cr", []byte("\r"), []string{""}},
		{"two newlines", []byte("\n\n"), []string{"", ""}}, // "\n\n" -> split ["", "", ""] -> trailing removed -> ["", ""]
		{"two crlf", []byte("\r\n\r\n"), []string{"", ""}},
		{"text then two newlines", []byte("text\n\n"), []string{"text", ""}},
		{"text then crlf crlf", []byte("text\r\n\r\n"), []string{"text", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapter.SplitLines(tt.content); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DefaultFileSystemAdapter.SplitLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultFileSystemAdapter_JoinLinesWithNewlines(t *testing.T) {
	adapter := NewDefaultFileSystemAdapter()
	tests := []struct {
		name  string
		lines []string
		want  []byte
	}{
		{"empty slice", []string{}, []byte("")},
		{"single line", []string{"hello"}, []byte("hello")},
		{"multiple lines", []string{"line1", "line2", "line3"}, []byte("line1\nline2\nline3")},
		{"lines with empty strings", []string{"line1", "", "line3"}, []byte("line1\n\nline3")},
		{"single empty string", []string{""}, []byte("")},                // string.Join([""], "\n") is ""
		{"multiple empty strings", []string{"", "", ""}, []byte("\n\n")}, // string.Join(["", "", ""], "\n") is "\n\n"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapter.JoinLinesWithNewlines(tt.lines); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DefaultFileSystemAdapter.JoinLinesWithNewlines() = %q, want %q", string(got), string(tt.want))
			}
		})
	}
}

func TestDefaultFileSystemAdapter_DetectLineEnding(t *testing.T) {
	adapter := NewDefaultFileSystemAdapter()
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{"lf", []byte("a\nb"), "\n"},
		{"crlf", []byte("a\r\nb"), "\r\n"},
		{"cr", []byte("a\rb"), "\r"},
		{"none", []byte("abc"), "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapter.DetectLineEnding(tt.content); got != tt.want {
				t.Errorf("DetectLineEnding() = %q, want %q", got, tt.want)
			}
		})
	}
}
