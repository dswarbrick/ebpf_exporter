//go:build linux
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

	"github.com/iovisor/gobpf/bcc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"gopkg.in/alecthomas/kingpin.v2"
)

const namespace = "ebpf"

func main() {
	toolkitFlags := kingpinflag.AddFlags(kingpin.CommandLine, ":9123")
	promlogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("ebpf_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promslog.New(promlogConfig)

	logger.Info("Starting ebpf_exporter", "version", version.Info())
	logger.Info("Build context", version.BuildContext())

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
			if err := m.AttachKprobe(fnName, kp, 10); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to attach %q to %q: %s\n", kpName, fnName, err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Failed to load %q: %s\n", kpName, err)
			os.Exit(1)
		}
	}

	prometheus.MustRegister(newExporter(m))

	landingConfig := web.LandingConfig{
		Name:        "eBPF Exporter",
		Description: "eBPF Exporter",
		Version:     version.Info(),
		Links: []web.LandingLinks{
			{
				Address: "/metrics",
				Text:    "Metrics",
			},
		},
	}
	landingPage, err := web.NewLandingPage(landingConfig)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	http.Handle("/", landingPage)
	http.Handle("/metrics", promhttp.Handler())

	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
