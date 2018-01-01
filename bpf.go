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

// NOTE: This BPF program currently causes gobpf's Module.load() function to
// fail with an ENOSPC error, due to a too small logbuf slice. See
// https://github.com/iovisor/gobpf/issues/103

package main

const bpfSource string = `
#include <uapi/linux/ptrace.h>
#include <linux/blkdev.h>
#include <linux/blk_types.h>

typedef struct disk_key {
	char disk[DISK_NAME_LEN];
	u64 slot;
} disk_key_t;

// Hash to temporily hold the start time of each bio request
BPF_HASH(start, struct request *);

// Histograms to separately record latencies of read / write requests
BPF_HISTOGRAM(read_lat, disk_key_t, 32);
BPF_HISTOGRAM(write_lat, disk_key_t, 32);

// Histograms to separately record sizes of read / write requests
BPF_HISTOGRAM(read_req_sz, disk_key_t, 16);
BPF_HISTOGRAM(write_req_sz, disk_key_t, 16);

// Record start time of a request
int trace_req_start(struct pt_regs *ctx, struct request *req)
{
	u64 ts = bpf_ktime_get_ns();
	start.update(&req, &ts);
	return 0;
}

// Calculate request duration and store in appropriate histogram bucket
int trace_req_completion(struct pt_regs *ctx, struct request *req, unsigned int bytes)
{
	u64 *tsp, delta;

	// Fetch timestamp and calculate delta
	tsp = start.lookup(&req);
	if (tsp == 0) {
		return 0;   // missed issue
	}
	delta = bpf_ktime_get_ns() - *tsp;

	// Convert to microseconds
	delta /= 1000;

	// Latency histogram key
	disk_key_t lat_key = {.slot = bpf_log2l(delta)};
	bpf_probe_read(&lat_key.disk, sizeof(lat_key.disk), req->rq_disk->disk_name);

	// Request size histogram key
	disk_key_t req_sz_key = {.slot = bpf_log2(bytes / 1024)};
	bpf_probe_read(&req_sz_key.disk, sizeof(req_sz_key.disk), req->rq_disk->disk_name);

	if ((req->cmd_flags & REQ_OP_MASK) == REQ_OP_WRITE) {
		write_lat.increment(lat_key);
		write_req_sz.increment(req_sz_key);
	} else {
		read_lat.increment(lat_key);
		read_req_sz.increment(req_sz_key);
	}

	start.delete(&req);
	return 0;
}
`
