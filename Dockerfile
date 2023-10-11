FROM golang:alpine
LABEL org.opencontainers.image.authors="daniel.swarbrick@gmail.com"
LABEL org.opencontainers.image.source="https://github.com/dswarbrick/ebpf_exporter"
LABEL org.opencontainers.image.title="ebpf_exporter"
LABEL org.opencontainers.image.description="ebpf_exporter is an experimental Prometheus exporter which uses eBPF kprobes to efficiently record a histogram of Linux bio request latencies and sizes"
RUN apk add --update --no-cache \
            bcc-dev \
            bcc-doc \
            bcc-tools \
            elfutils-libelf \
            gcc \
            git \
            libgcc \
            libstdc++ \
            linux-headers \
            musl-dev \
            zlib
WORKDIR $GOPATH/src/github.com/dswarbrick/ebpf_exporter
COPY . .
RUN go mod download -x
RUN go install -ldflags="-s -w" . 
EXPOSE 9123/tcp
CMD ["/go/bin/ebpf_exporter"]
