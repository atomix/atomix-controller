// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"fmt"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	"github.com/go-logr/logr"
)

// NewControllerLogger returns a new implementation of the Kubernetes Logger interface
func NewControllerLogger(names ...string) logr.Logger {
	return &ControllerLogger{
		log: logging.GetLogger(names...),
	}
}

// ControllerLogger is an implementation of the Kubernetes controller Logger interface
type ControllerLogger struct {
	log logging.Logger
}

func getFields(keysAndValues ...interface{}) []logging.Field {
	fields := make([]logging.Field, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		key := keysAndValues[i]
		value := keysAndValues[i+1]
		field := logging.String(fmt.Sprint(key), fmt.Sprint(value))
		fields = append(fields, field)
	}
	return fields
}

// Enabled returns whether the logger is enabled
func (l *ControllerLogger) Enabled() bool {
	return true
}

// Info logs an info level message
func (l *ControllerLogger) Info(msg string, keysAndValues ...interface{}) {
	l.log.WithFields(getFields(keysAndValues...)...).Info(msg)
}

// Error logs an error level message
func (l *ControllerLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.log.WithFields(getFields(keysAndValues...)...).Error(err, msg)
}

// V sets the verbosity level
func (l *ControllerLogger) V(level int) logr.InfoLogger {
	return l
}

// WithValues sets the log values
func (l *ControllerLogger) WithValues(keysAndValues ...interface{}) logr.Logger {
	return &ControllerLogger{
		log: l.log.WithFields(getFields(keysAndValues...)...),
	}
}

// WithName sets the logger name
func (l *ControllerLogger) WithName(name string) logr.Logger {
	return &ControllerLogger{
		log: l.log.GetLogger(name),
	}
}
