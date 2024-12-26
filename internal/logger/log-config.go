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
    "flag"
    log "github.com/sirupsen/logrus"
    "os"
    "gopkg.in/natefinch/lumberjack.v2"
    "fmt"
    "strings"
    "bytes"
    "path/filepath"
    ec "log_exporter/internal/utils/errorcodes"
)

const (
    TEXT = "text"
    JSON = "json"
    CLOUD = "cloud"
)

var (
    logLevel       = logLevelFlag(log.DebugLevel)
    logFile        = flag.String("log-path", "", "Redirect log output to file (stdout if empty)")
    logFormat      = flag.String("log-format", TEXT, fmt.Sprintf("Log format : %v (default), %v or %v", TEXT, CLOUD, JSON))
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
    createDirForFile(*logFile)
    if *logFile != "" {
        lf, err := os.OpenFile(*logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644);
        if err != nil {
            log.
                WithField("logFile", *logFile).
                Fatal("unable to create or truncate log file")
        }
        if (!*logRotation) {
            log.SetOutput(lf)
            fmt.Printf("Log rotation disabled: logFile=%v\n", *logFile)
        } else {            
            log.SetOutput(&lumberjack.Logger{
                Filename:   *logFile,
                MaxSize:    *logMaxSize,
                MaxBackups: *logMaxBackups,
                MaxAge:     *logMaxAge,
                Compress:   *logArchivation,
            })
            fmt.Printf("Log rotation enabled: logFile=%v, logMaxSize=%v MB, logMaxBackups=%v, logMaxAge=%v, logArchivation=%v, logLevel=%v\n", *logFile, *logMaxSize, *logMaxBackups, *logMaxAge, *logArchivation, logLevel)
        }
    }
    if *logFormat == JSON {
        log.SetFormatter(&log.JSONFormatter{})
    } else if *logFormat == CLOUD {
        log.SetReportCaller(true)
        log.SetFormatter(&CloudFormatter{})
    }
    log.RegisterExitHandler(func() {
        log.Error("fatal error occurred, exit log-exporter")
    })
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

    var fullName, name string
    if entry.Caller != nil {
        fullName = entry.Caller.File
        names := strings.Split(fullName, "/")
        index := len(names) - 1
        if index >= 0 {
            name = names[index]
        }
    }
    b.WriteString(name)
    b.WriteString("] ")
    errorCode := entry.Data[ec.FIELD]
    if errorCode != nil {
        b.WriteString("[")
        b.WriteString(ec.FIELD)
        b.WriteByte('=')
        b.WriteString(fmt.Sprintf("%v", errorCode))
        b.WriteString("] ")
    }
    b.WriteString(entry.Message)
    b.WriteByte('\n')
    return b.Bytes(), nil
}

func createDirForFile(filePath string) {
    dir := filepath.Dir(filePath)
    if dir == "" || dir == "." {
        return
    }
    err := os.MkdirAll(dir, 0755)
    if err != nil {
        fmt.Printf("Error creating directory %v : %+v", dir, err)
    }
}