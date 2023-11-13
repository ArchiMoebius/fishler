package util

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
)

func TestGetProfileBuffer(t *testing.T) {
	data, err := GetProfileBuffer("tester")

	if err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(bytes.NewReader(data))

	var found_group = false
	var found_passwd = false
	var done = false

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			done = true

		// return any other error
		case err != nil:
			t.Fatal(err)

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		if done {
			break
		}

		// the target location where the dir/file should be created
		if header.Name == "group" {
			found_group = true
		}

		if header.Name == "passwd" {
			found_passwd = true
		}
		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()
	}

	if found_group == false {
		t.Fatal("Failed to locate group file in tar for profile")
	}

	if found_passwd == false {
		t.Fatal("Failed to locate passwd file in tar for profile")
	}
}
