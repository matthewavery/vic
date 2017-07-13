// Copyright 2017 VMware, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nfs

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vmware/vic/lib/archive"
	"github.com/vmware/vic/pkg/trace"
)

// MountDataSink implements the DataSink interface for mounted devices
type NFSDataSink struct {
	NfsTarget  *Target
	volumePath string
	Clean      func()
}

func (n *NFSDataSink) Sink() interface{} {
	// XXX: we should really return the volume path and the target here...
	return n.NfsTarget
}

func (n *NFSDataSink) Import(op trace.Operation, spec *archive.FilterSpec, data io.ReadCloser) error {
	return unpackUsingTarget(op, spec, data, n.NfsTarget, n.volumePath)
}

func (n *NFSDataSink) Close() error {
	// maybe accomodate for errors in the "Clean" function in the future. since there
	// are many potential points of failure.
	n.Clean()
	return nil
}

// unpackUsingTarget will unpack the given tarstream(if it is a tar stream) onto the target nfs filesystem based on the volumePath.
//
// the pathSpec will include the following elements
// - include : any tar entry that has a path below(after stripping) the include path will be written
// - strip : The strip string will indicate the
// - exlude : marks paths that are to be excluded from the write operation
// - rebase : marks the the write path that will be tacked onto the "volumePath". e.g /tmp/unpack + /my/target/path = /tmp/unpack/my/target/path
// NOTE: this is a heavily influenced by the lib/archive/unpack.go unpack implementation
func unpackUsingTarget(op trace.Operation, spec *archive.FilterSpec, tarStream io.ReadCloser, nfsTarget *Target, volumePath string) error {
	// the tar stream should be wrapped up at the end of this call
	tr := tar.NewReader(tarStream)

	strip := spec.StripPath
	target := spec.RebasePath

	if target == "" {
		op.Debugf("Bad target path in FilterSpec (%#v)", spec)
		return fmt.Errorf("Invalid write target specified")
	}

	if strip == "" {
		op.Debugf("Strip path was set to \"\"")
	}

	if _, _, err := nfsTarget.Lookup(volumePath); err != nil {
		// the target unpack path does not exist. We should not get here.
		op.Errorf("tar unpack target does not exist (%s)", volumePath)
		return err
	}

	finalTargetPath := filepath.Join(volumePath, target)
	op.Debugf("finalized target path for Tar unpack operation at (%s)", finalTargetPath)

	// process the tarball onto the filesystem
	for {
		header, err := tr.Next()
		if err == io.EOF {
			// This indicates the end of the archive
			break
		}
		if err != nil {
			// it is likely in this case that we were not given a legitimate tar stream
			op.Debugf("Received error (%s) when attempting a tar read operation on target stream", err)
			return err
		}

		// skip excluded elements unless explicitly included
		if archive.Excluded(header.Name, spec) {
			continue
		}

		// fix up path
		strippedTargetPath := strings.TrimPrefix(header.Name, strip)
		writePath := filepath.Join(finalTargetPath, strippedTargetPath)

		switch header.Typeflag {
		case tar.TypeDir:
			err = mkdirAll(writePath, header.FileInfo().Mode(), nfsTarget)
			if err != nil {
				return err
			}
			continue
		case tar.TypeSymlink:
			op.Infof("Found symlink target, nfs does not support nfs targets at this time (%s)", header.FileInfo().Name())
			fallthrough
		default:
			// we will treat the default as a regular file
			targetDir, _ := filepath.Split(writePath)

			// FIXME: this is a hack we must include the directory before this instead of excluding it. since the permissions could be different.
			err = mkdirAll(writePath, header.FileInfo().Mode(), nfsTarget)
			if err != nil {
				// bail for now instead of skipping.
				return err
			}

			err = createRegularFile(writePath, fileWriteFlags, header.FileInfo().Mode(), tr, target)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// createRegularFile will use the provided nfs target to create the target file
func createRegularFile(path string, flags int, perm os.FileMode, tr *tar.Reader, target *Target) error {

	// notes:
	// 1. check if file exists
	// 2. if exists remove. ---> not 100% on this... if we bail out we have actually removed data from the filesystem at this point(not ideal).
	// 3. OpenFile
	// 4. write the contents.
	// 5. close the file

	info, _, err := nfsTarget.Lookup(path)
	if err == nil {
		if info.IsDir() {
			// cannot overwrite directory with non directory
			// the question is do we bail or skip?
			return fmt.Errorf("Cannot overwrite directory with non directory at path (%s)", path)
		}
		// since we cannot tell the nfs client to truncate we must remove the file
		err = target.Remove(path)
		if err != nil {
			return err
		}
	}

	if !os.IsNotExist(err) {
		return err
	}

	writtenTarFile, err := target.OpenFile(path, perm)
	if err != nil {
		return nil
	}
	defer writtenTarFile.Close()

	_, err = io.Copy(writtenTarFile, tr)
	if err != nil {
		return err
	}
	return nil
}

// mkdirAll is a utility function that takes the provided target and creates all targets along a specified path
func mkdirAll(path string, mode os.FileMode, target *Target) error {
	var dirsToCreate []string
	currentPath := path
	parent, dirName := filepath.Split(currentPath)

	// some light sanity checking
	if !mode.IsDir() {
		return fmt.Errorf("caller supplied non dir based file mode")
	}

	// ensure target does not already exist.
	info, _, err := target.Lookup(currentPath)
	if err == nil {
		if !info.IsDir() {
			// only return ERREXIST if the target is not a directory
			return os.ErrExist
		}
		return nil
	}
	currentPath = parent
	dirsToCreate = append(dirsToCreate, dirName)

	// collect dirs that are missing
	for {
		if currentPath == "/" {
			//the entire path did not exist
			break
		}

		info, _, err := target.Lookup(currentPath)
		if err == nil {
			if !info.IsDir() {
				// only return ERREXIST if the target is not a directory
				return fmt.Errorf("target (%s) along mkdirall target path (%s) exists as a non directory", currentPath, path)
			}
			break
		}

		currentPath, dirToAdd := filepath.Split(currentPath)
		if os.IsNotExist(err) {
			// append to the front
			dirsToCreate = append([]string{dirToAdd}, dirsToCreate...)
		} else {
			// something else happened and we should bail
			return err
		}

	}

	// now create the needed directories in order
	for _, dir := range dirsToCreate {
		currentPath = filepath.Join(currentPath, dir)
		_, err := target.Mkdir(currentPath, info.Mode())

		// bail on error here
		if err != nil {
			return err
		}
	}

	return nil
}
