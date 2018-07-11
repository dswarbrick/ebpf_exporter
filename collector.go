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
	latTableLen   = 28 // Must match max_io_lat_slot in BPF program.
	reqSzTableLen = 16 // Must match max_io_req_sz_slot in BPF program.

	// Linux req_opf enums, cf. linux/blk_types.h
	REQ_OP_READ         = 0
	REQ_OP_WRITE        = 1
	REQ_OP_FLUSH        = 2
	REQ_OP_DISCARD      = 3
	REQ_OP_ZONE_REPORT  = 4
	REQ_OP_SECURE_ERASE = 5
	REQ_OP_ZONE_RESET   = 6
	REQ_OP_WRITE_SAME   = 7
	REQ_OP_WRITE_ZEROES = 9
	REQ_OP_SCSI_IN      = 32
	REQ_OP_SCSI_OUT     = 33
	REQ_OP_DRV_IN       = 34
	REQ_OP_DRV_OUT      = 35
)

var (
	// Map of request operation enums to human-readable strings. This map does not include all
	// possible request operations, but covers the most commonly observed ones.
	reqOpStrings = map[uint8]string{
		REQ_OP_READ:         "read",
		REQ_OP_WRITE:        "write",
		REQ_OP_FLUSH:        "flush",
		REQ_OP_DISCARD:      "discard",
		REQ_OP_WRITE_SAME:   "write_same",
		REQ_OP_WRITE_ZEROES: "write_zeroes",
	}
)

type exporter struct {
	bpfMod       *bcc.Module
	ioLat        *bcc.Table
	ioReqSz      *bcc.Table
	latency      *prometheus.Desc
	reqSize      *prometheus.Desc
	tableEntries *prometheus.GaugeVec
}

func newExporter(m *bcc.Module) *exporter {
	e := exporter{
		bpfMod:  m,
		ioLat:   bcc.NewTable(m.TableId("io_lat"), m),
		ioReqSz: bcc.NewTable(m.TableId("io_req_sz"), m),
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
		tableEntries: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "bio",
				Name:      "bpf_table_entries",
				Help:      "The number of BPF table entries used.",
			},
			[]string{"table"},
		),
	}

	prometheus.MustRegister(e.tableEntries)

	return &e
}

func (e *exporter) Collect(ch chan<- prometheus.Metric) {
	var (
		n   uint
		tbl map[string]map[uint8][]uint64
	)

	n, tbl = decodeTable(e.ioLat, latTableLen)
	e.tableEntries.WithLabelValues("req_latency").Set(float64(n))
	e.emit(ch, e.latency, tbl)

	n, tbl = decodeTable(e.ioReqSz, reqSzTableLen)
	e.tableEntries.WithLabelValues("req_size").Set(float64(n))
	e.emit(ch, e.reqSize, tbl)
}

func (e *exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.latency
	ch <- e.reqSize
}

func (e *exporter) emit(ch chan<- prometheus.Metric, hist *prometheus.Desc, devBuckets map[string]map[uint8][]uint64) {
	for devName, reqs := range devBuckets {
		for reqOp, bpfBuckets := range reqs {
			var (
				count uint64
				sum   float64
			)

			// Skip unrecognized request operations.
			if _, ok := reqOpStrings[reqOp]; !ok {
				continue
			}

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
				devName, reqOpStrings[reqOp],
			)
		}
	}
}

// decodeTable decodes a BPF table and returns a per-device map of values as ordered buckets.
func decodeTable(table *bcc.Table, tableSize uint) (uint, map[string]map[uint8][]uint64) {
	var numEntries uint

	devBuckets := make(map[string]map[uint8][]uint64)

	// bcc.Table.Iter() returns unsorted entries, so write the decoded values into an order-
	// preserving slice.
	for it := table.Iter(); it.Next(); {
		keyStr, _ := table.KeyBytesToStr(it.Key())
		devName, op, bucket := parseKey(keyStr)

		// First time seeing this device, create map for request operations
		if _, ok := devBuckets[devName]; !ok {
			devBuckets[devName] = make(map[uint8][]uint64)
		}

		// First time seeing this req op for this device, create slice for buckets
		if _, ok := devBuckets[devName][op]; !ok {
			devBuckets[devName][op] = make([]uint64, tableSize)
		}

		valueStr, _ := table.LeafBytesToStr(it.Leaf())

		// entry.Value is a hexadecimal string, e.g., 0x1f3
		if value, err := strconv.ParseUint(valueStr, 0, 64); err == nil {
			devBuckets[devName][op][bucket] = value
		}

		numEntries++
	}

	return numEntries, devBuckets
}

// parseKey parses a BPF hash key as created by the BPF program. For example:
// `{ "sda" 0x1 0xb }` is the key for the 11th bucket for a write operation on device "sda".
func parseKey(s string) (string, uint8, uint64) {
	fields := strings.Fields(strings.Trim(s, "{ }"))
	devName := strings.Trim(fields[0], "\"")
	reqOp, _ := strconv.ParseUint(fields[1], 0, 64)
	bucket, _ := strconv.ParseUint(fields[2], 0, 64)
	return devName, uint8(reqOp), bucket
}
