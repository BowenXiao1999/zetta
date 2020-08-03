// Copyright 2020 Zhizhesihai (Beijing) Technology Limited.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	FetchRowsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "fetch_rows_total",
			Help:      "Counter of fetchRows.",
		}, []string{LblType})

	FetchSparseCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "fetch_sparse_total",
			Help:      "Counter of fetchSparse.",
		}, []string{LblType})

	FetchRowsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "fetch_rows_duration_seconds",
			Help:      "Bucketed histogram of processing time (s) in running table read-store.",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 22), // 100us ~ 419s
		}, []string{LblType})

	FetchSparseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "fetch_sparse_duration_seconds",
			Help:      "Bucketed histogram of processing time (s) in running mutate executor.",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 22), // 100us ~ 419s
		}, []string{LblType})

	BatchSparseCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "batch_sparse_total",
			Help:      "Counter of batchSparse.",
		}, []string{LblType})

	ScanSparseCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "scan_sparse_total",
			Help:      "Counter of scanSparse.",
		}, []string{LblType})

	BatchSparseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "batch_sparse_duration_seconds",
			Help:      "Bucketed histogram of processing time (s) in running mutate executor.",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 22), // 100us ~ 419s
		}, []string{LblType})

	ScanSparseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "zetta",
			Subsystem: "tables",
			Name:      "scan_sparse_duration_seconds",
			Help:      "Bucketed histogram of processing time (s) in running mutate executor.",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 22), // 100us ~ 419s
		}, []string{LblType})
)
