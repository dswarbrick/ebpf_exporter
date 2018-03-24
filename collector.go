// +build linux

// ebpf_exporter - A Prometheus exporter for Linux block IO statistics.
//
// Copyright 2018 Daniel Swarbrick
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"math"
	"strconv"
	"strings"

	"github.com/iovisor/gobpf/bcc"

	"github.com/prometheus/client_golang/prometheus"
)

type bioStats struct {
	readLat    [32]uint64 // Must match size of read_lat table in BPF program
	writeLat   [32]uint64 // Must match size of write_lat table in BPF program
	readReqSz  [16]uint64 // Must match size of read_req_sz table in BPF program
	writeReqSz [16]uint64 // Must match size of write_req_sz table in BPF program
}

type exporter struct {
	bpfMod     *bcc.Module
	readLat    *bcc.Table
	writeLat   *bcc.Table
	readReqSz  *bcc.Table
	writeReqSz *bcc.Table
	latency    *prometheus.Desc
	reqSize    *prometheus.Desc
}

func newExporter(m *bcc.Module) *exporter {
	e := exporter{
		bpfMod:     m,
		readLat:    bcc.NewTable(m.TableId("read_lat"), m),
		writeLat:   bcc.NewTable(m.TableId("write_lat"), m),
		readReqSz:  bcc.NewTable(m.TableId("read_req_sz"), m),
		writeReqSz: bcc.NewTable(m.TableId("write_req_sz"), m),
		latency: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "bio", "req_latency"),
			"A histogram of bio request latencies in microseconds.",
			[]string{"device", "operation"},
			nil,
		),
		reqSize: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "bio", "req_size"),
			"A histogram of bio request sizes in KiB.",
			[]string{"device", "operation"},
			nil,
		),
	}

	return &e
}

func (e *exporter) Collect(ch chan<- prometheus.Metric) {
	devStats := make(map[string]*bioStats)

	// Table.Iter() returns unsorted entries, so write the decoded values into an array in the
	// correct order.
	for entry := range e.readLat.Iter() {
		devName, bucket := parseKey(entry.Key)

		stats, ok := devStats[devName]
		if !ok {
			// First time seeing this device, initialize new bioStats struct / maps
			stats = new(bioStats)
			devStats[devName] = stats
		}

		// entry.Value is a hexadecimal string, e.g., 0x1f3
		if value, err := strconv.ParseUint(entry.Value, 0, 64); err == nil {
			stats.readLat[bucket] = value
		}
	}

	// FIXME: Eliminate duplicated code
	for entry := range e.writeLat.Iter() {
		devName, bucket := parseKey(entry.Key)

		stats, ok := devStats[devName]
		if !ok {
			// First time seeing this device, initialize new bioStats struct / maps
			stats = new(bioStats)
			devStats[devName] = stats
		}

		if value, err := strconv.ParseUint(entry.Value, 0, 64); err == nil {
			stats.writeLat[bucket] = value
		}
	}

	// FIXME: Eliminate duplicated code
	for entry := range e.readReqSz.Iter() {
		devName, bucket := parseKey(entry.Key)

		stats, ok := devStats[devName]
		if !ok {
			// First time seeing this device, initialize new bioStats struct / maps
			stats = new(bioStats)
			devStats[devName] = stats
		}

		if value, err := strconv.ParseUint(entry.Value, 0, 64); err == nil {
			stats.readReqSz[bucket] = value
		}
	}

	// FIXME: Eliminate duplicated code
	for entry := range e.writeReqSz.Iter() {
		devName, bucket := parseKey(entry.Key)

		stats, ok := devStats[devName]
		if !ok {
			// First time seeing this device, initialize new bioStats struct / maps
			stats = new(bioStats)
			devStats[devName] = stats
		}

		if value, err := strconv.ParseUint(entry.Value, 0, 64); err == nil {
			stats.writeReqSz[bucket] = value
		}
	}

	// Walk devStats map and emit metrics to channel
	for devName, stats := range devStats {
		emit := func(hist *prometheus.Desc, bpfBuckets []uint64, reqOp string) {
			var count uint64

			promBuckets := make(map[float64]uint64)

			// Prometheus histograms are cumulative, so count must be a running total of previous
			// buckets also.
			for k, v := range bpfBuckets {
				count += v
				promBuckets[math.Exp2(float64(k))] = count
			}

			ch <- prometheus.MustNewConstHistogram(hist,
				count,
				0,   // FIXME: sum
				promBuckets,
				devName, reqOp,
			)
		}

		emit(e.latency, stats.readLat[:], "read")
		emit(e.latency, stats.writeLat[:], "write")

		emit(e.reqSize, stats.readReqSz[:], "read")
		emit(e.reqSize, stats.writeReqSz[:], "write")
	}
}

func (e *exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.latency
	ch <- e.reqSize
}

// parseKey parses a BPF hash key as created by the BPF program. For example:
// `{ "sda" 0xb }` is the key for the 11th bucket for device "sda".
func parseKey(s string) (string, uint64) {
	fields := strings.Fields(strings.Trim(s, "{ }"))
	label := strings.Trim(fields[0], "\"")
	bucket, _ := strconv.ParseUint(fields[1], 0, 64)
	return label, bucket
}
