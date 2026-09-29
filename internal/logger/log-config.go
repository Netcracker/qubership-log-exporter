// Copyright 2024 Qubership
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"bytes"
	"flag"
	"fmt"
	ec "log_exporter/internal/utils/errorcodes"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// timestampFormat is the ISO-8601 layout the logging guide requires: yyyy-MM-dd'T'HH:mm:ss.SSSZ.
	timestampFormat = "2006-01-02T15:04:05.000Z07:00"

	TEXT  = "text"
	JSON  = "json"
	CLOUD = "cloud"
)

var (
	logLevel       = logLevelFlag(log.DebugLevel)
	logFile        = flag.String("log-path", "", "Redirect log output to file (stdout if empty)")
	logFormat      = flag.String("log-format", JSON, fmt.Sprintf("Log format : %v (default), %v or %v", JSON, CLOUD, TEXT))
	logRotation    = flag.Bool("log-rotation", true, "Enabling log rotation")
	logMaxSize     = flag.Int("log-max-size", 100, "Set max log size in Mb which triggers rotation")
	logMaxBackups  = flag.Int("log-max-backups", 20, "Set max number of backups")
	logMaxAge      = flag.Int("log-max-age", 90, "Set max age of log backups in days")
	logArchivation = flag.Bool("log-archivation", true, "Archivation for rotated logs")
)

func (level *logLevelFlag) Set(value string) error {
	if lvl, err := log.ParseLevel(value); err != nil {
		return err
	} else {
		*level = logLevelFlag(lvl)
	}
	return nil
}

type logLevelFlag log.Level

func (level logLevelFlag) String() string {
	return log.Level(level).String()
}

func init() {
	flag.Var(&logLevel, "log-level", "Log level")
}

func ConfigureLog() {
	log.SetLevel(log.Level(logLevel))
	// The formatter is installed first, so that every line below already carries the
	// configured layout, including the ones written while the log file is being set up.
	setFormatter()
	createDirForFile(*logFile)
	if *logFile != "" {
		lf, err := os.OpenFile(*logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			log.
				WithField("log_file", *logFile).
				Fatal("unable to create or truncate log file")
		}
		if !*logRotation {
			log.SetOutput(lf)
			log.WithField("log_file", *logFile).Info("Log rotation is disabled")
		} else {
			log.SetOutput(&lumberjack.Logger{
				Filename:   *logFile,
				MaxSize:    *logMaxSize,
				MaxBackups: *logMaxBackups,
				MaxAge:     *logMaxAge,
				Compress:   *logArchivation,
			})
			log.WithFields(log.Fields{
				"log_file":         *logFile,
				"log_max_size_mb":  *logMaxSize,
				"log_max_backups":  *logMaxBackups,
				"log_max_age_days": *logMaxAge,
				"log_archivation":  *logArchivation,
				"log_level":        logLevel.String(),
			}).Info("Log rotation is enabled")
		}
	}
	log.RegisterExitHandler(func() {
		log.Error("fatal error occurred, exit log-exporter")
	})
}

func setFormatter() {
	switch *logFormat {
	case JSON:
		// The caller feeds the class field, the JSON counterpart of the class marker
		// that the cloud format prints.
		log.SetReportCaller(true)
		log.SetFormatter(&utcFormatter{&log.JSONFormatter{
			TimestampFormat: timestampFormat,
			FieldMap: log.FieldMap{
				log.FieldKeyTime:  "time",
				log.FieldKeyLevel: "level",
				log.FieldKeyMsg:   "message",
				log.FieldKeyFile:  "class",
			},
			CallerPrettyfier: func(frame *runtime.Frame) (string, string) {
				// An empty function name drops the func key; only class is wanted.
				return "", shortFileName(frame.File)
			},
		}})
	case CLOUD:
		log.SetReportCaller(true)
		log.SetFormatter(&CloudFormatter{})
	}
}

// utcFormatter normalizes the entry clock to UTC, which the logging guide requires,
// and then delegates the rendering to the wrapped formatter.
type utcFormatter struct {
	log.Formatter
}

func (f *utcFormatter) Format(entry *log.Entry) ([]byte, error) {
	utcEntry := *entry
	utcEntry.Time = entry.Time.UTC()
	return f.Formatter.Format(&utcEntry)
}

// shortFileName reduces a full source path to its base file name.
func shortFileName(fullName string) string {
	names := strings.Split(fullName, "/")
	index := len(names) - 1
	if index < 0 {
		return ""
	}
	return names[index]
}

type CloudFormatter struct {
	log.TextFormatter
}

func (f *CloudFormatter) Format(entry *log.Entry) ([]byte, error) {
	b := &bytes.Buffer{}
	b.WriteByte('[')
	b.WriteString(entry.Time.Format("2006-01-02T15:04:05.000"))
	b.WriteString("] [")
	b.WriteString(entry.Level.String())
	b.WriteString("] [x_request_id=0] [tenant_id=-] [thread=-] [class=")

	var name string
	if entry.Caller != nil {
		name = shortFileName(entry.Caller.File)
	}
	b.WriteString(name)
	b.WriteString("] ")
	// error_code keeps its leading position; every other structured field follows in a
	// stable order, so that the cloud format carries the same data as the JSON format.
	if errorCode := entry.Data[ec.FIELD]; errorCode != nil {
		writeCloudField(b, ec.FIELD, errorCode)
	}
	keys := make([]string, 0, len(entry.Data))
	for key := range entry.Data {
		if key != ec.FIELD {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeCloudField(b, key, entry.Data[key])
	}
	b.WriteString(entry.Message)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

func writeCloudField(b *bytes.Buffer, key string, value interface{}) {
	b.WriteByte('[')
	b.WriteString(key)
	b.WriteByte('=')
	// A field value may span several lines; the text format keeps every entry on one line.
	b.WriteString(strings.ReplaceAll(fmt.Sprintf("%v", value), "\n", "\\n"))
	b.WriteString("] ")
}

func createDirForFile(filePath string) {
	dir := filepath.Dir(filePath)
	if dir == "" || dir == "." {
		return
	}
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.WithFields(log.Fields{"dir": dir, "error": err}).Error("Cannot create the log directory")
	}
}
