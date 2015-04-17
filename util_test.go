// test utility functions
package main

import (
	"testing"
)

func TestFilenameRenamingAbsolute(t *testing.T) {
	in := "/abc/def/ghi.MAT"
	tgt := "/jkl/mno/ghi.MAT"
	if out := MovedFilePath(in, "/jkl/mno"); out != tgt {
		t.Logf("rename failed: %s != %s.\n", out, tgt)
		t.Fail()
	}
}

func TestFilenameRenamingRelative(t *testing.T) {
	in := "ghi.MAT"
	tgt := "/jkl/mno/ghi.MAT"
	if out := MovedFilePath(in, "/jkl/mno"); out != tgt {
		t.Logf("rename failed: %s != %s.\n", out, tgt)
		t.Fail()
	}
}

func TestRelativeMoveWithBaseDir(t *testing.T) {
	in := "/abc/def/foo.bar"
	base := "/abc"
	tgt := "/ghi"
	expect := "/ghi/def/foo.bar"

	if out, _ := RenamePathRelativeTo(in, base, tgt); out != expect {
		t.Logf("rename relative failed: %s != %s", out, expect)
		t.Fail()
	}
}

func TestRelativeMoveNoBase(t *testing.T) {
	in := "/abc/def/foo.bar"
	base := "/ghi"
	tgt := "/jkl"

	if _, err := RenamePathRelativeTo(in, base, tgt); err == nil {
		t.Logf("should have gotten error with mismatched base path")
		t.Fail()
	}
}

func TestRelativeMoveNoSubdir(t *testing.T) {
	in := "/abc/def/foo.bar"
	base := "/abc/def/"
	tgt := "/jkl"
	expect := "/jkl/foo.bar"

	if out, _ := RenamePathRelativeTo(in, base, tgt); out != expect {
		t.Logf("rename relative failed with no subdir: %s != %s", out, expect)
		t.Fail()
	}
}

func TestRelativeMoveNoTrailingSlash(t *testing.T) {
	in := "/abc/def/foo.bar"
	base := "/abc/def"
	tgt := "/jkl"
	expect := "/jkl/foo.bar"

	if out, _ := RenamePathRelativeTo(in, base, tgt); out != expect {
		t.Logf("rename relative failed with no trailing slash: %s != %s",
			out, expect)
		t.Fail()
	}
}
