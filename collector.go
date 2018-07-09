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

const (
	latTableLen   = 32 // Must match size of read_lat / write_lat tables in BPF program
	reqSzTableLen = 16 // Must match size of read_req_sz / write_req_sz tables in BPF program
)

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
	emit := func(hist *prometheus.Desc, devBuckets map[string][]uint64, reqOp string) {
		for devName, bpfBuckets := range devBuckets {
			var (
				count uint64
				sum   float64
			)

			promBuckets := make(map[float64]uint64)

			for k, v := range bpfBuckets {
				// Prometheus histograms are cumulative, so count must be a running total of
				// previous buckets also.
				count += v

				// Sum will not be completely accurate, since the BPF program already discarded
				// some resolution when storing occurrences of values in log2 buckets. Count and
				// sum are required however to calculate an average from a histogram.
				exp2 := math.Exp2(float64(k))
				sum += exp2 * float64(v)

				promBuckets[exp2] = count
			}

			ch <- prometheus.MustNewConstHistogram(hist,
				count,
				sum,
				promBuckets,
				devName, reqOp,
			)
		}
	}

	emit(e.latency, decodeTable(e.readLat, latTableLen), "read")
	emit(e.latency, decodeTable(e.writeLat, latTableLen), "write")

	emit(e.reqSize, decodeTable(e.readReqSz, reqSzTableLen), "read")
	emit(e.reqSize, decodeTable(e.writeReqSz, reqSzTableLen), "write")
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

// decodeTable decodes a BPF table and returns a per-device map of values as ordered buckets.
func decodeTable(table *bcc.Table, tableSize uint) map[string][]uint64 {
	devBuckets := make(map[string][]uint64)

	// bcc.Table.Iter() returns unsorted entries, so write the decoded values into an order-
	// preserving slice.
	for entry := range table.Iter() {
		devName, bucket := parseKey(entry.Key)

		// First time seeing this device, create slice for buckets
		if _, ok := devBuckets[devName]; !ok {
			devBuckets[devName] = make([]uint64, tableSize)
		}

		// entry.Value is a hexadecimal string, e.g., 0x1f3
		if value, err := strconv.ParseUint(entry.Value, 0, 64); err == nil {
			// FIXME? Possibly hitting "index out of range" if bucket > (tableSize - 1)
			devBuckets[devName][bucket] = value
		}
	}

	return devBuckets
}
