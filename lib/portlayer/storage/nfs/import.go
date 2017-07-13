// Copyright 2016-2017 VMware, Inc. All Rights Reserved.
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
	"io"

	"github.com/vmware/vic/lib/archive"
	"github.com/vmware/vic/lib/portlayer/storage"
	"github.com/vmware/vic/pkg/trace"
)

func (v *VolumeStore) Import(op trace.Operation, id string, spec *archive.FilterSpec, tarstream io.ReadCloser) error {
	l, err := v.NewDataSink(op, id)
	if err != nil {
		return err
	}

	return l.Import(op, spec, tarstream)
}

// NewDataSink creates and returns a DataSink associated with nfs volume storage
func (v *VolumeStore) NewDataSink(op trace.Operation, id string) (storage.DataSink, error) {
	target, err := v.Service.Mount(op)
	if err != nil {
		return nil, err
	}

	cleanFunc = func() {
		err := v.Service.Unmount(op)
		op.Errorf("Received error while attempting to unmount nfs volume store: (%s)", err)
	}

	sink := NFSDataSink{
		NfsTarget:  *target,
		volumePath: v.volDirPath(id),
		Clean:      cleanFunc,
	}

	return sink, nil
}
