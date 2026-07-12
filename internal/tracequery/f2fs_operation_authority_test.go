package tracequery

import "testing"

func TestFileOperationFromEventNameF2FSWriteAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "f2fs_write_begin", want: "write"},
		{name: "f2fs_write_end", want: "write"},
		{name: "f2fs_sync_file_enter", want: "sync"},
		{name: "vendor_f2fs_write_begin", want: ""},
		{name: "f2fs_write_begin_extra", want: ""},
		{name: " f2fs_write_begin", want: ""},
		{name: "f2fs_write_begin ", want: ""},
		{name: "F2FS_write_begin", want: ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := fileOperationFromEventName(test.name); got != test.want {
				t.Fatalf("fileOperationFromEventName(%q)=%q, want %q", test.name, got, test.want)
			}
		})
	}
}
