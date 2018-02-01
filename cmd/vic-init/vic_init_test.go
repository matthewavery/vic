// Copyright 2016-2018 VMware, Inc. All Rights Reserved.
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

package main

import (
	"flag"
	"os"
	"testing"

	log "github.com/Sirupsen/logrus"

	"github.com/vmware/vic/lib/install/management"
	"github.com/vmware/vic/lib/system"
	"github.com/vmware/vic/lib/tether"
	"github.com/vmware/vic/pkg/trace"
)

var (
	systemTest *bool
)

func init() {
	systemTest = flag.Bool("systemtest", false, "Set to true when running system tests")
}

// TestMain simply so we have control of debugging level and somewhere to call package wide test setup
func TestMain(m *testing.M) {

	if !*systemTest {
		log.SetLevel(log.DebugLevel)
		trace.Logger = log.StandardLogger()

		// replace the Sys variable with a mock
		tether.Sys = system.System{
			Hosts:      &tether.MockHosts{},
			ResolvConf: &tether.MockResolvConf{},
			Syscall:    &tether.MockSyscall{},
			Root:       os.TempDir(),
		}
	} else {
		management.PortlayerArgs = append(management.PortlayerArgs, "--systemtest=true")
		management.PortlayerArgs = append(management.PortlayerArgs, "--test.coverprofile port-layer-server.cov")

	}

	retCode := m.Run()

	// call with result of m.Run()
	os.Exit(retCode)

}
