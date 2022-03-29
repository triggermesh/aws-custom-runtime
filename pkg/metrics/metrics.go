/*
Copyright 2022 Triggermesh Inc.

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

package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/kelseyhightower/envconfig"

	"contrib.go.opencensus.io/exporter/prometheus"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// Environment variables realted to the metrics exporter.
type env struct {
	// Name of the component that the current function belongs to.
	Name string `envconfig:"name" default:""`
	// Namespace where the component is running.
	Namespace string `envconfig:"namespace" default:""`
	// Component is the higher level resource type that runs the function.
	Component string `envconfig:"k_component" default:"function"`
	// ResourceGroup of the component that the current function belongs to.
	ResourceGroup string `envconfig:"resource_group" default:"functions.extensions.triggermesh.io"`
	// PrometheusPort is the port number where metrics exporter will be running
	PrometheusPort string `envconfig:"metrics_prometheus_port" default:"9092"`
}

// Names for exported metrics.
const (
	metricNameEventProcessingSuccessCount = "event_processing_success_count"
	metricNameEventProcessingErrorCount   = "event_processing_error_count"
	metricNameEventProcessingLatencies    = "event_processing_latencies"
)

// Tags for exported metrics.
var (
	tagKeyName           = tag.MustNewKey("name")
	tagKeyResourceGroup  = tag.MustNewKey("resource_group")
	tagKeyNamespace      = tag.MustNewKey("namespace_name")
	tagKeyEventType      = tag.MustNewKey("event_type")
	tagKeyEventSource    = tag.MustNewKey("event_source")
	tagKeyUserManagedErr = tag.MustNewKey("user_managed")
)

// eventProcessingSuccessCountM is a measure of the number of events that were
// successfully processed by a component.
var eventProcessingSuccessCountM = stats.Int64(
	metricNameEventProcessingSuccessCount,
	"Number of events successfully processed by the Function",
	stats.UnitDimensionless,
)

// eventProcessingSuccessCountM is a measure of the number of events that were
// unsuccessfully processed by a component.
var eventProcessingErrorCountM = stats.Int64(
	metricNameEventProcessingErrorCount,
	"Number of events unsuccessfully processed by the Function",
	stats.UnitDimensionless,
)

// eventProcessingLatenciesM is a measure of the time spent by a component
// processing events.
var eventProcessingLatenciesM = stats.Int64(
	metricNameEventProcessingLatencies,
	"Time spent in the Function handler processing events",
	stats.UnitMilliseconds,
)

// registerEventProcessingStatsView registers an OpenCensus stats view for
// metrics related to events processing, and panics in case of error.
func registerEventProcessingStatsView() error {
	commonTagKeys := []tag.Key{
		tagKeyName,
		tagKeyResourceGroup,
		tagKeyNamespace,
		tagKeyEventType,
		tagKeyEventSource,
	}

	return view.Register(
		&view.View{
			Measure:     eventProcessingSuccessCountM,
			Description: eventProcessingSuccessCountM.Description(),
			Aggregation: view.Count(),
			TagKeys:     commonTagKeys,
		},
		&view.View{
			Measure:     eventProcessingErrorCountM,
			Description: eventProcessingErrorCountM.Description(),
			Aggregation: view.Count(),
			TagKeys: append(commonTagKeys,
				tagKeyUserManagedErr,
			),
		},
		&view.View{
			Measure:     eventProcessingLatenciesM,
			Description: eventProcessingLatenciesM.Description(),
			Aggregation: view.Distribution(0, 10, 20, 30, 40, 50, 100, 200, 500, 1000, 2000, 5000, 10000),
			TagKeys:     commonTagKeys,
		},
	)
}

// EventProcessingStatsReporter collects and reports stats about the processing of events.
type EventProcessingStatsReporter struct {
	// context that holds pre-populated OpenCensus tags
	tagsCtx context.Context
}

// ReportProcessingSuccess increments eventProcessingSuccessCountM.
func (r *EventProcessingStatsReporter) ReportProcessingSuccess(tms ...tag.Mutator) {
	tagsCtx, _ := tag.New(r.tagsCtx, tms...)
	stats.Record(tagsCtx, eventProcessingSuccessCountM.M(1))
}

// ReportProcessingError increments eventProcessingErrorCountM.
func (r *EventProcessingStatsReporter) ReportProcessingError(userManaged bool, tms ...tag.Mutator) {
	tms = append(tms,
		tag.Insert(tagKeyUserManagedErr, strconv.FormatBool(userManaged)),
	)

	tagsCtx, _ := tag.New(r.tagsCtx, tms...)
	stats.Record(tagsCtx, eventProcessingErrorCountM.M(1))
}

// ReportProcessingLatency records in eventProcessingLatenciesM the processing
// duration of an event.
func (r *EventProcessingStatsReporter) ReportProcessingLatency(d time.Duration, tms ...tag.Mutator) {
	tagsCtx, _ := tag.New(r.tagsCtx, tms...)
	stats.Record(tagsCtx, eventProcessingLatenciesM.M(d.Milliseconds()))
}

// StatsExporter registers metric views and starts the exporter.
func StatsExporter() (*EventProcessingStatsReporter, error) {
	var env env
	if err := envconfig.Process("", &env); err != nil {
		log.Fatalf("Cannot process metrics env variables: %v", err)
	}

	registerEventProcessingStatsView()

	ctx, err := tag.New(context.Background(),
		tag.Insert(tagKeyResourceGroup, env.ResourceGroup),
		tag.Insert(tagKeyNamespace, env.Namespace),
		tag.Insert(tagKeyName, env.Name),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating OpenCensus tags: %w", err)
	}

	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: env.Component,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create the Prometheus stats exporter: %w", err)
	}
	go func() {
		metricsExporter := http.NewServeMux()
		metricsExporter.Handle("/metrics", pe)
		if err := http.ListenAndServe(":"+env.PrometheusPort, metricsExporter); err != nil {
			log.Fatalf("Failed to run Prometheus scrape endpoint: %v", err)
		}
	}()

	return &EventProcessingStatsReporter{
		tagsCtx: ctx,
	}, nil
}
