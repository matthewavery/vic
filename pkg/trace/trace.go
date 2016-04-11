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

package trace

import (
	"runtime"
	"time"

	log "github.com/Sirupsen/logrus"
)

//This logger is compatible with the standard lib logger.
var defaultLogger = *log.New()

//Begin : begin trace logging function with a provided standard logger.
func Begin(msg string) (string, string, time.Time) {
	return logBegin(msg, defaultLogger)
}

//BeginLogger : begin trace logging function that allows user to provide their logger.
func BeginLogger(msg string, logger log.Logger) (string, string, time.Time) {
	return logBegin(msg, logger)
}

//helper function that abstracts out the actual beginning of the trace logging.
func logBegin(msg string, logger log.Logger) (string, string, time.Time) {
	pc, _, _, _ := runtime.Caller(1)
	name := runtime.FuncForPC(pc).Name()

	if msg == "" {
		logger.Printf("[BEGIN] [%s]", name)
	} else {
		logger.Printf("[BEGIN] [%s] %s", name, msg)
	}
	return msg, name, time.Now()

}

//End : end trace logging function with a provided standard logger.
func End(msg string, name string, startTime time.Time) {
	logEnd(msg, name, startTime, defaultLogger)
}

//EndLogger : end trace logging function that allows user to provide their logger.
func EndLogger(msg string, name string, startTime time.Time, logger log.Logger) {
	logEnd(msg, name, startTime, logger)
}

//helper function that abstracts out the actual ending of the trace logging.
func logEnd(msg string, name string, startTime time.Time, logger log.Logger) {
	endTime := time.Now()
	logger.Printf("[ END ] [%s] [%s] %s", name, endTime.Sub(startTime), msg)
}
