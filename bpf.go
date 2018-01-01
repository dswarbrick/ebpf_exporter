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

const bpfSource string = `
#include <uapi/linux/ptrace.h>
#include <linux/blkdev.h>
#include <linux/blk_types.h>

typedef struct disk_key {
	char disk[DISK_NAME_LEN];
	u64 slot;
} disk_key_t;

BPF_HASH(start, struct request *);
BPF_HISTOGRAM(read_lat, disk_key_t, 32);
BPF_HISTOGRAM(write_lat, disk_key_t, 32);

// Record start time of a request
int trace_req_start(struct pt_regs *ctx, struct request *req)
{
	u64 ts = bpf_ktime_get_ns();
	start.update(&req, &ts);
	return 0;
}

// Calculate request duration and store in appropriate histogram bucket
int trace_req_completion(struct pt_regs *ctx, struct request *req)
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

	// Store as histogram
	disk_key_t key = {.slot = bpf_log2l(delta)};
	bpf_probe_read(&key.disk, sizeof(key.disk), req->rq_disk->disk_name);

	if ((req->cmd_flags & REQ_OP_MASK) == REQ_OP_WRITE) {
		write_lat.increment(key);
	} else {
		read_lat.increment(key);
	}

	start.delete(&req);
	return 0;
}
`
