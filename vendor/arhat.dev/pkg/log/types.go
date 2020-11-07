/*
Copyright 2019 The arhat.dev Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package log

type Interface interface {
	// Enabled check if level is enabled
	Enabled(level Level) bool

	// WithName return a logger which shares config and runtime but with different name
	WithName(name string) Interface

	// WithFields return a logger with predefined fields
	WithFields(fields ...Field) Interface

	// V is the verbose level, should be used in library and not useful or untested stuff
	// such as showing a call started, printing some internal values for inspection
	V(msg string, fields ...Field)

	// D is the debug level, should be used for information
	D(msg string, fields ...Field)

	// I is the info level, should be used to indicate application state, show important messages
	I(msg string, fields ...Field)

	// E is the error level, should only be used to report unexpected or fatal error, will print
	// a stacktrace when used
	E(msg string, fields ...Field)

	// Flush logger
	Flush() error
}

type Structure struct {
	Msg        string `json:"M,omitempty"`
	Level      string `json:"L,omitempty"`
	Time       string `json:"T,omitempty"`
	Name       string `json:"N,omitempty"`
	Caller     string `json:"C,omitempty"`
	Stacktrace string `json:"S,omitempty"`
}
