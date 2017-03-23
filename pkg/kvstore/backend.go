// Copyright 2016 VMware, Inc. All Rights Reserved.
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

package kvstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"

	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/vic/pkg/retry"
	"github.com/vmware/vic/pkg/vsphere/datastore"
)

type Backend interface {
	// Save saves data to the specified path
	Save(ctx context.Context, r io.Reader, path string) error
	// Load loads data from the specified path
	Load(ctx context.Context, path string) (io.ReadCloser, error)
}

func NewDatastoreBackend(ds *datastore.Helper) Backend {
	return &dsBackend{ds: ds}
}

type dsBackend struct {
	ds *datastore.Helper
}

// Save saves data to the specified path
func (d *dsBackend) Save(ctx context.Context, r io.Reader, path string) error {
	// upload to an ephemeral file
	tmpfile := fmt.Sprintf("%s.tmp", path)
	if err := d.ds.Upload(ctx, r, tmpfile); err != nil {
		return err
	}
	log.Debugf("kv store upload of file (%s) was successful", tmpfile)

	if err := d.ds.Mv(ctx, tmpfile, path); err != nil {
		log.Debugf("failed to move file during kv store save operation at path (%s) to path (%s) with error (%s)", tmpfile, path, err.Error())

		// The below case can occur on VSAN in slow environments(especially nested).
		// we will reattempt the move since it might take some time for the upload to replicate before presenting.
		if isFileFault(err) {
			log.Debugf("FileNotFound fault returned during a kv store move operation. retrying move operation. tmpfile (%s), path (%s)", tmpfile, path)

			moveOperation := func() error {
				return d.ds.Mv(ctx, tmpfile, path)
			}
			if err := retry.Do(moveOperation, isFileFault); err != nil {
				log.Debugf("failed to move file (%s) to (%s) after attempting to recover from a FileNotFoundFault with error (%s) during a kv store save operation.", tmpfile, path, err.Error())
				return err
			}
			return nil
		}
		return err
	}

	return nil
}

func toOsError(err error) error {
	switch err.Error() {
	case fmt.Sprintf("%d %s", http.StatusNotFound, http.StatusText(http.StatusNotFound)):
		return os.ErrNotExist
	}

	return err
}

// Load loads data from the specified path
func (d *dsBackend) Load(ctx context.Context, path string) (io.ReadCloser, error) {
	rc, err := d.ds.Download(ctx, path)
	if err != nil {
		return nil, toOsError(err)
	}
	log.Debugf("kv store download of file (%s) was successful", path)

	return rc, err
}

func isFileFault(err error) bool {
	if soap.IsVimFault(err) {
		switch soap.ToVimFault(err).(type) {
		case *types.FileNotFound:
			return true
		default:
			return false
		}
	}
	return false
}
