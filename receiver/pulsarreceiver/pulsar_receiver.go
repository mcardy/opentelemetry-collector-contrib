// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pulsarreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/pulsarreceiver"

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.uber.org/zap"
)

var errUnrecognizedEncoding = errors.New("unrecognized encoding")

const alreadyClosedError = "AlreadyClosedError"

type pulsarTracesConsumer struct {
	id              config.ComponentID
	tracesConsumer  consumer.Traces
	topic           string
	client          pulsar.Client
	cancel          context.CancelFunc
	consumer        pulsar.Consumer
	unmarshaler     TracesUnmarshaler
	settings        component.ReceiverCreateSettings
	consumerOptions pulsar.ConsumerOptions
}

func newTracesReceiver(config Config, set component.ReceiverCreateSettings, unmarshalers map[string]TracesUnmarshaler, nextConsumer consumer.Traces) (*pulsarTracesConsumer, error) {
	unmarshaler := unmarshalers[config.Encoding]
	if nil == unmarshaler {
		return nil, errUnrecognizedEncoding
	}

	options := config.clientOptions()
	client, err := pulsar.NewClient(options)
	if err != nil {
		return nil, err
	}

	consumerOptions, err := config.consumerOptions()
	if err != nil {
		return nil, err
	}

	return &pulsarTracesConsumer{
		id:              config.ID(),
		tracesConsumer:  nextConsumer,
		topic:           config.Topic,
		unmarshaler:     unmarshaler,
		settings:        set,
		client:          client,
		consumerOptions: consumerOptions,
	}, nil
}

func (c *pulsarTracesConsumer) Start(context.Context, component.Host) error {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	_consumer, err := c.client.Subscribe(c.consumerOptions)
	if err == nil {
		c.consumer = _consumer
		go func() {
			if e := consumerTracesLoop(ctx, c); e != nil {
				c.settings.Logger.Error("consume traces loop occurs an error", zap.Error(e))
			}
		}()
	}

	return err
}

func consumerTracesLoop(ctx context.Context, c *pulsarTracesConsumer) error {
	unmarshaler := c.unmarshaler
	traceConsumer := c.tracesConsumer

	for {
		message, err := c.consumer.Receive(ctx)
		if err != nil {
			if strings.Contains(err.Error(), alreadyClosedError) {
				return err
			}
			if errors.Is(err, context.Canceled) {
				c.settings.Logger.Info("exiting consume traces loop")
				return err
			}
			c.settings.Logger.Error("failed to receive traces message from Pulsar, waiting for one second before retrying", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		traces, err := unmarshaler.Unmarshal(message.Payload())
		if err != nil {
			c.settings.Logger.Error("failed to unmarshaler traces message", zap.Error(err))
			c.consumer.Ack(message)
			return err
		}

		if err := traceConsumer.ConsumeTraces(context.Background(), traces); err != nil {
			c.settings.Logger.Error("consume traces failed", zap.Error(err))
		}
		c.consumer.Ack(message)
	}
}

func (c *pulsarTracesConsumer) Shutdown(context.Context) error {
	c.cancel()
	c.consumer.Close()
	c.client.Close()
	return nil
}

type pulsarMetricsConsumer struct {
	id              config.ComponentID
	metricsConsumer consumer.Metrics
	unmarshaler     MetricsUnmarshaler
	topic           string
	client          pulsar.Client
	consumer        pulsar.Consumer
	cancel          context.CancelFunc
	settings        component.ReceiverCreateSettings
	consumerOptions pulsar.ConsumerOptions
}

func newMetricsReceiver(config Config, set component.ReceiverCreateSettings, unmarshalers map[string]MetricsUnmarshaler, nextConsumer consumer.Metrics) (*pulsarMetricsConsumer, error) {
	unmarshaler := unmarshalers[config.Encoding]
	if nil == unmarshaler {
		return nil, errUnrecognizedEncoding
	}

	options := config.clientOptions()
	client, err := pulsar.NewClient(options)
	if err != nil {
		return nil, err
	}

	consumerOptions, err := config.consumerOptions()
	if err != nil {
		return nil, err
	}

	return &pulsarMetricsConsumer{
		id:              config.ID(),
		metricsConsumer: nextConsumer,
		topic:           config.Topic,
		unmarshaler:     unmarshaler,
		settings:        set,
		client:          client,
		consumerOptions: consumerOptions,
	}, nil
}

func (c *pulsarMetricsConsumer) Start(context.Context, component.Host) error {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	_consumer, err := c.client.Subscribe(c.consumerOptions)
	if err == nil {
		c.consumer = _consumer

		go func() {
			if e := consumeMetricsLoop(ctx, c); e != nil {
				c.settings.Logger.Error("consume metrics loop occurs an error", zap.Error(e))
			}
		}()
	}

	return err
}

func consumeMetricsLoop(ctx context.Context, c *pulsarMetricsConsumer) error {
	unmarshaler := c.unmarshaler
	metricsConsumer := c.metricsConsumer

	for {
		message, err := c.consumer.Receive(ctx)
		if err != nil {
			if strings.Contains(err.Error(), alreadyClosedError) {
				return err
			}
			if errors.Is(err, context.Canceled) {
				c.settings.Logger.Info("exiting consume metrics loop")
				return err
			}

			c.settings.Logger.Error("failed to receive metrics message from Pulsar, waiting for one second before retrying", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		metrics, err := unmarshaler.Unmarshal(message.Payload())
		if err != nil {
			c.settings.Logger.Error("failed to unmarshaler metrics message", zap.Error(err))
			c.consumer.Ack(message)
			return err
		}

		if err := metricsConsumer.ConsumeMetrics(context.Background(), metrics); err != nil {
			c.settings.Logger.Error("consume traces failed", zap.Error(err))
		}

		c.consumer.Ack(message)
	}
}

func (c *pulsarMetricsConsumer) Shutdown(context.Context) error {
	c.cancel()
	c.consumer.Close()
	c.client.Close()
	return nil
}

type pulsarLogsConsumer struct {
	id              config.ComponentID
	logsConsumer    consumer.Logs
	unmarshaler     LogsUnmarshaler
	topic           string
	client          pulsar.Client
	consumer        pulsar.Consumer
	cancel          context.CancelFunc
	settings        component.ReceiverCreateSettings
	consumerOptions pulsar.ConsumerOptions
}

func newLogsReceiver(config Config, set component.ReceiverCreateSettings, unmarshalers map[string]LogsUnmarshaler, nextConsumer consumer.Logs) (*pulsarLogsConsumer, error) {
	unmarshaler := unmarshalers[config.Encoding]
	if nil == unmarshaler {
		return nil, errUnrecognizedEncoding
	}

	options := config.clientOptions()
	client, err := pulsar.NewClient(options)
	if err != nil {
		return nil, err
	}

	consumerOptions, err := config.consumerOptions()
	if err != nil {
		return nil, err
	}

	return &pulsarLogsConsumer{
		id:              config.ID(),
		logsConsumer:    nextConsumer,
		topic:           config.Topic,
		cancel:          nil,
		unmarshaler:     unmarshaler,
		settings:        set,
		client:          client,
		consumerOptions: consumerOptions,
	}, nil
}

func (c *pulsarLogsConsumer) Start(context.Context, component.Host) error {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	_consumer, err := c.client.Subscribe(c.consumerOptions)
	if err == nil {
		c.consumer = _consumer
		go func() {
			if e := consumeLogsLoop(ctx, c); e != nil {
				c.settings.Logger.Error("consume logs loop occurs an error", zap.Error(e))
			}
		}()
	}

	return err
}

func consumeLogsLoop(ctx context.Context, c *pulsarLogsConsumer) error {
	unmarshaler := c.unmarshaler
	logsConsumer := c.logsConsumer

	for {
		message, err := c.consumer.Receive(ctx)
		if err != nil {
			if strings.Contains(err.Error(), alreadyClosedError) {
				return err
			}
			if errors.Is(err, context.Canceled) {
				c.settings.Logger.Info("exiting consume traces loop canceled")
				return err
			}
			c.settings.Logger.Error("failed to receive logs message from Pulsar, waiting for one second before retrying", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		logs, err := unmarshaler.Unmarshal(message.Payload())
		if err != nil {
			c.settings.Logger.Error("failed to unmarshaler logs message", zap.Error(err))
			c.consumer.Ack(message)
			return err
		}

		if err := logsConsumer.ConsumeLogs(context.Background(), logs); err != nil {
			c.settings.Logger.Error("consume traces failed", zap.Error(err))
		}

		c.consumer.Ack(message)
	}
}

func (c *pulsarLogsConsumer) Shutdown(context.Context) error {
	c.cancel()
	c.consumer.Close()
	c.client.Close()
	return nil
}
