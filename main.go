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
	"fmt"
	"net/http"
	"os"

	"github.com/go-kit/kit/log/level"
	"github.com/iovisor/gobpf/bcc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const namespace = "ebpf"

var (
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9123").String()
)

func main() {
	allowedLevel := promlog.AllowedLevel{}
	flag.AddFlags(kingpin.CommandLine, &allowedLevel)
	kingpin.Version(version.Print("ebpf_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(allowedLevel)

	level.Info(logger).Log("msg", "Starting ebpf_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", version.BuildContext())

	// Compile BPF code and return new module
	m := bcc.NewModule(bpfSource, []string{})
	defer m.Close()

	// Map of kprobe names from our BPF program to kernel function names, to which to attach.
	kprobes := map[string]string{
		"trace_req_start":      "blk_account_io_start",
		"trace_req_completion": "blk_account_io_completion",
	}

	// Load kprobes and attach them to kernel functions
	for kpName, fnName := range kprobes {
		if kp, err := m.LoadKprobe(kpName); err == nil {
			if err := m.AttachKprobe(fnName, kp); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to attach %q to %q: %s\n", kpName, fnName, err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Failed to load %q: %s\n", kpName, err)
			os.Exit(1)
		}
	}

	prometheus.MustRegister(newExporter(m))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>
	<head>
	<title>eBPF Exporter</title>
	<style>html { font-family: sans-serif; }</style>
	</head>
	<body>
	<h1>eBPF Exporter</h1>
	<p><a href="/metrics">Metrics</a></p>
	</body>
</html>`))
	})
	http.Handle("/metrics", promhttp.Handler())

	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
		os.Exit(1)
	}
}
