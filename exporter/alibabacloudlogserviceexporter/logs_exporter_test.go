// Copyright 2020, OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package alibabacloudlogserviceexporter

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"
)

func createSimpleLogData(numberOfLogs int) plog.Logs {
	logs := plog.NewLogs()
	logs.ResourceLogs().AppendEmpty() // Add an empty ResourceLogs
	rl := logs.ResourceLogs().AppendEmpty()
	rl.ScopeLogs().AppendEmpty() // Add an empty ScopeLogs
	sl := rl.ScopeLogs().AppendEmpty()

	for i := 0; i < numberOfLogs; i++ {
		ts := pcommon.Timestamp(int64(i) * time.Millisecond.Nanoseconds())
		logRecord := sl.LogRecords().AppendEmpty()
		logRecord.Body().SetStr("mylog")
		logRecord.Attributes().PutStr(conventions.AttributeServiceName, "myapp")
		logRecord.Attributes().PutStr("my-label", "myapp-type")
		logRecord.Attributes().PutStr(conventions.AttributeHostName, "myhost")
		logRecord.Attributes().PutStr("custom", "custom")
		logRecord.SetTimestamp(ts)
	}
	sl.LogRecords().AppendEmpty()

	return logs
}

func TestNewLogsExporter(t *testing.T) {
	got, err := newLogsExporter(componenttest.NewNopExporterCreateSettings(), &Config{
		ExporterSettings: config.NewExporterSettings(config.NewComponentID(typeStr)),
		Endpoint:         "us-west-1.log.aliyuncs.com",
		Project:          "demo-project",
		Logstore:         "demo-logstore",
	})
	assert.NoError(t, err)
	require.NotNil(t, got)

	// This will put trace data to send buffer and return success.
	err = got.ConsumeLogs(context.Background(), createSimpleLogData(3))
	assert.NoError(t, err)
	time.Sleep(time.Second * 4)
}

func TestSTSTokenExporter(t *testing.T) {
	got, err := newLogsExporter(componenttest.NewNopExporterCreateSettings(), &Config{
		ExporterSettings: config.NewExporterSettings(config.NewComponentID(typeStr)),
		Endpoint:         "us-west-1.log.aliyuncs.com",
		Project:          "demo-project",
		Logstore:         "demo-logstore",
		TokenFilePath:    filepath.Join("testdata", "config.yaml"),
	})
	assert.NoError(t, err)
	require.NotNil(t, got)
}

func TestNewFailsWithEmptyLogsExporterName(t *testing.T) {
	got, err := newLogsExporter(componenttest.NewNopExporterCreateSettings(), &Config{})
	assert.Error(t, err)
	require.Nil(t, got)
}
