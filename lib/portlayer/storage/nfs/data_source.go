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
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/vmware/vic/lib/archive"
	"github.com/vmware/vic/pkg/trace"
)

// NFSDataSource implements the DataSource interface for nfs based volumes
type NFSDataSource struct {
	NfsTarget  Target
	volumePath string
	Clean      func()
}

func (n *NFSDataSource) Source() interface{} {
	// we will need to return both the target and the volume path.
	return n.NfsTarget
}

func (n *NFSDataSource) Export(op trace.Operation, spec *archive.FilterSpec, data bool) (io.ReadCloser, error) {
	// data will be ignored for now
	return packUsingTarget(op, n.volumePath, spec, data)
}

// packUsingTarget will pack up contents that are targeted through the pathspec and return them in a tar archive.
// NOTE: this could be generalized and the Target interface used for other things besides NFS in the future.
//       if generalized we should work to ensure that only OS path/file errors are returned so that the builtin
//       can be utilized for error checking.
//
//      - also in the future this might be called RemoteDataSource when generalized.
func (n *NFSDataSource) packUsingTarget(op trace.Operation, dir string, spec *FilterSpec, data bool) (io.ReadCloser, error) {
	// for now data is ignored and always returned.
	data = true

	var (
		err error
		hdr *tar.Header
	)

	// Note: it is up to the caller to handle errors and the closing of the read side of the pipe
	r, w := io.Pipe()
	go func() {
		tw := tar.NewWriter(w)
		defer func() {
			var cerr error
			if cerr = tw.Close(); cerr != nil {
				op.Errorf("Error closing tar writer: %s", cerr.Error())
			}
			if err == nil {
				_ = w.CloseWithError(cerr)
			} else {
				_ = w.CloseWithError(err)
			}
			return
		}()

		targetPath := findReadTarget(spec)
		readPath := filepath.Join(dir, targetPath)

		_, err := n.NfsTarget.Lookup(readPath)
		if err {
			op.Errorf("Lookup of target (%s) returned an error", readPath)
		}

		// assemble the readList
		readList := []string{readPath}

		// churn through the read list until we have nothing left to read.
		for len(readList) > 0 {

			currentRead := readList[0]

			if Excluded(currentRead, spec) {
				// skip this entry and remove the target from the readList
				readList = append(readList[:0], readList[1:]...)
				continue
			}

			// NOTE: there is a race condition between looking up the file for the header write, and actually copying the contents over.

			info, _, err := n.NfsTarget.Lookup(currentRead)
			if err != nil {
				// It is possible that after a target is added to the readList
				// that it is removed by another actor on the fs.
				// In this case we remove the element and continue on.
				readList = append(readList[:0], readList[1:]...)
				continue
			}

			// write the header
			// NOTE: nfs target does not support a ReadLink style functionality yet, so we cannot provide link targets for symlinks.
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				// In the past we have bailed out when this happens, so we will here as well.
				op.Debugf("Encountered error creating header for target (%s) : (%s)", currentRead, err)
				return err
			}

			if hdr.Typeflag == tar.TypeDir {
				hdr.Name += "/"

				// now we will Read the directory contents and add them to the list.
				directoryChildren, err := n.NfsTarget.ReadDir(currentRead)
				if err != nil {
					// we could not read this directory, at this point we should fail due to the inabilitiy to complete some portion of the read.
					op.Debugf("Unable to read directory (%s) : %s", currentRead, err)
					return err
				}

				for _, childInfo := range directoryChildren {
					readList = append(readList, childInfo.Name())
				}

			}

			err = tw.WriteHeader(hdr)
			if err != nil {
				op.Debugf("Unable to write assembled header for (%s) : %s", hdr.Name, err)
				// we do not really handle this in the disk case... so we will only log it here as well.
			}

			if hdr.Typeflag == tar.TypeReg && hdr.Size != 0 {

				// perform read in a closure
				err := func() error {
					f, err = n.NfsTarget.Open(currentRead)
					if err != nil {
						if os.IsPermission(err) {
							err = nil
						}
						return
					}
					if f != nil {
						_, err = io.Copy(tw, f)
						if err != nil {
							op.Errorf("Error writing archive data: %s", err.Error())
						}
						_ = f.Close()
					}

				}()
			}
		}
	}()
	return r, err

}

// findReadTarget will return the shortest key in the Inclusion map
func findReadTarget(spec *archive.FilterSpec) string {
	// currently an off the cuff implementation(probably could make better with more time)
	mapLength := len(spec.Inclusions)
	if mapLength == 0 {
		return "/"
	}

	paths := make([]string, 0, len(spec.Inclusions))
	for path := range spec.Inclusions {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths[0]
}
