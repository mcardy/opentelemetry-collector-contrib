// Copyright 2021 OpenTelemetry Authors
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

package tcplogreceiver

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/consumer/consumertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/input/tcp"
)

func TestTcp(t *testing.T) {
	testTCP(t, testdataConfigYaml())
}

func testTCP(t *testing.T, cfg *TCPLogConfig) {
	numLogs := 5

	f := NewFactory()
	sink := new(consumertest.LogsSink)
	rcvr, err := f.CreateLogsReceiver(context.Background(), componenttest.NewNopReceiverCreateSettings(), cfg, sink)
	require.NoError(t, err)
	require.NoError(t, rcvr.Start(context.Background(), componenttest.NewNopHost()))

	var conn net.Conn
	conn, err = net.Dial("tcp", "127.0.0.1:29018")
	require.NoError(t, err)

	for i := 0; i < numLogs; i++ {
		msg := fmt.Sprintf("<86>1 2021-02-28T00:0%d:02.003Z test msg %d\n", i, i)
		_, err = conn.Write([]byte(msg))
		require.NoError(t, err)
	}
	require.NoError(t, conn.Close())

	require.Eventually(t, expectNLogs(sink, numLogs), 2*time.Second, time.Millisecond)
	require.NoError(t, rcvr.Shutdown(context.Background()))
	require.Len(t, sink.AllLogs(), 1)

	resourceLogs := sink.AllLogs()[0].ResourceLogs().At(0)
	logs := resourceLogs.ScopeLogs().At(0).LogRecords()

	for i := 0; i < numLogs; i++ {
		log := logs.At(i)

		msg := log.Body()
		require.Equal(t, msg.Str(), fmt.Sprintf("<86>1 2021-02-28T00:0%d:02.003Z test msg %d", i, i))
	}
}

func TestLoadConfig(t *testing.T) {
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	sub, err := cm.Sub("tcplog")
	require.NoError(t, err)
	require.NoError(t, config.UnmarshalReceiver(sub, cfg))

	assert.NoError(t, cfg.Validate())
	assert.Equal(t, testdataConfigYaml(), cfg)
}

func testdataConfigYaml() *TCPLogConfig {
	return &TCPLogConfig{
		BaseConfig: adapter.BaseConfig{
			ReceiverSettings: config.NewReceiverSettings(config.NewComponentID(typeStr)),
			Operators:        []operator.Config{},
			Converter: adapter.ConverterConfig{
				WorkerCount: 1,
			},
		},
		InputConfig: func() tcp.Config {
			c := tcp.NewConfig()
			c.ListenAddress = "127.0.0.1:29018"
			return *c
		}(),
	}
}

func TestDecodeInputConfigFailure(t *testing.T) {
	factory := NewFactory()
	badCfg := &TCPLogConfig{
		BaseConfig: adapter.BaseConfig{
			ReceiverSettings: config.NewReceiverSettings(config.NewComponentID(typeStr)),
			Operators:        []operator.Config{},
		},
		InputConfig: func() tcp.Config {
			c := tcp.NewConfig()
			c.Encoding.Encoding = "fake"
			return *c
		}(),
	}
	receiver, err := factory.CreateLogsReceiver(context.Background(), componenttest.NewNopReceiverCreateSettings(), badCfg, consumertest.NewNop())
	require.Error(t, err, "receiver creation should fail if input config isn't valid")
	require.Nil(t, receiver, "receiver creation should fail if input config isn't valid")
}

func expectNLogs(sink *consumertest.LogsSink, expected int) func() bool {
	return func() bool {
		return sink.LogRecordCount() == expected
	}
}
