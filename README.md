# ebpf_exporter

ebpf_exporter is an experimental Prometheus exporter which uses eBPF kprobes to efficiently record
a histogram of Linux bio request latencies and sizes.

## Sample Output

Linux bio request latencies for each block device are recorded in log2 buckets separately for read
and write, in microseconds. This should cover use cases ranging from high speed flash-based
devices, to legacy HDD devices.

```
# HELP ebpf_bio_req_latency A histogram of bio request latencies in microseconds.
# TYPE ebpf_bio_req_latency histogram
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="64"} 1
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="128"} 629
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="256"} 261
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="512"} 10
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="1024"} 1
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="2048"} 11
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="4096"} 6
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="8192"} 1
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="16384"} 7
ebpf_bio_req_latency_bucket{device="sda",operation="read",le="+Inf"} 927
ebpf_bio_req_latency_sum{device="sda",operation="read"} 323520
ebpf_bio_req_latency_count{device="sda",operation="read"} 927
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="64"} 1
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="128"} 21
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="256"} 83
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="512"} 82
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="1024"} 79
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="2048"} 314
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="4096"} 736
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="8192"} 327
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="16384"} 38
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="32768"} 9
ebpf_bio_req_latency_bucket{device="sda",operation="write",le="+Inf"} 1690
ebpf_bio_req_latency_sum{device="sda",operation="write"} 7.400896e+06
ebpf_bio_req_latency_count{device="sda",operation="write"} 1690
```

Request sizes (in KiB) are also recorded in log2 buckets for each device:

```
# HELP ebpf_bio_req_size A histogram of bio request sizes in KiB.
# TYPE ebpf_bio_req_size histogram
ebpf_bio_req_size_bucket{device="sda",operation="read",le="4"} 879
ebpf_bio_req_size_bucket{device="sda",operation="read",le="8"} 17
ebpf_bio_req_size_bucket{device="sda",operation="read",le="16"} 15
ebpf_bio_req_size_bucket{device="sda",operation="read",le="32"} 5
ebpf_bio_req_size_bucket{device="sda",operation="read",le="64"} 9
ebpf_bio_req_size_bucket{device="sda",operation="read",le="128"} 2
ebpf_bio_req_size_bucket{device="sda",operation="read",le="+Inf"} 927
ebpf_bio_req_size_sum{device="sda",operation="read"} 4884
ebpf_bio_req_size_count{device="sda",operation="read"} 927
ebpf_bio_req_size_bucket{device="sda",operation="write",le="4"} 975
ebpf_bio_req_size_bucket{device="sda",operation="write",le="8"} 144
ebpf_bio_req_size_bucket{device="sda",operation="write",le="16"} 72
ebpf_bio_req_size_bucket{device="sda",operation="write",le="32"} 123
ebpf_bio_req_size_bucket{device="sda",operation="write",le="64"} 83
ebpf_bio_req_size_bucket{device="sda",operation="write",le="128"} 39
ebpf_bio_req_size_bucket{device="sda",operation="write",le="256"} 98
ebpf_bio_req_size_bucket{device="sda",operation="write",le="512"} 146
ebpf_bio_req_size_bucket{device="sda",operation="write",le="1024"} 10
ebpf_bio_req_size_bucket{device="sda",operation="write",le="+Inf"} 1690
ebpf_bio_req_size_sum{device="sda",operation="write"} 130524
ebpf_bio_req_size_count{device="sda",operation="write"} 1690
```

Note that immediately after starting the exporter, not all bucket sizes will be shown. As soon as a
request latency / size occurs which would land in a specific bucket, that bucket will appear in the
output. The application used to graph the data should be able to handle non-contiguous buckets.

## Grafana Panel Samples

Grafana does not currently support Prometheus data sources properly for heatmaps, which is the
ultimate goal of this exporter. Support for the feature is expected to land in Grafana 5.0 (see
https://github.com/grafana/grafana/issues/10009). In the meantime, it is possible to create stacked
bar charts which show a breakdown request latency / size over time:

![IO request latency](img/disk-io-request-latency.png)

![IO request size](img/disk-io-request-size.png)
