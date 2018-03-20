FROM alpine:edge

RUN apk add --update \
  git \
  llvm-dev \
  llvm-static \
  clang-dev \
  clang-static \
  cmake \
  flex-dev \
  bison \
  luajit-dev \
  build-base \
  iperf \
  linux-headers \
  elfutils-dev \
  zlib-dev \
  python-dev

RUN ln -s /usr/lib/cmake/llvm5/ /usr/lib/cmake/llvm; \
    ln -s /usr/include/llvm5/llvm-c/ /usr/include/llvm-c; \
    ln -s /usr/include/llvm5/llvm/ /usr/include/llvm

RUN git clone https://github.com/iovisor/bcc.git

WORKDIR /bcc

# Specific patches
RUN git config --global user.email "build@example.com" && \
    git checkout v0.5.0 && \
    git cherry-pick -m 1 b44d705657d24a54605e10da1bd92a2d8b13b908 && \
    git cherry-pick -m 1 3dbb0db486b155fb2ce6850d8d9c69bd9974a0db && \
    git cherry-pick -m 1 04ec1fa84a669dbf7f48728237f8d24c32a38803

WORKDIR /bcc/build

RUN cmake .. -DCMAKE_INSTALL_PREFIX=/usr -DCMAKE_BUILD_TYPE=Release
RUN make -j$(nproc)
RUN make install
RUN echo -e "#include <unistd.h>\n$(cat /usr/include/bcc/libbpf.h)" > /usr/include/bcc/libbpf.h
RUN strip /usr/lib64/libbcc.so.0.5.0

FROM golang:1.10-alpine
RUN wget -O /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.4.1/dep-linux-amd64 && chmod +x /usr/local/bin/dep
RUN apk add --update --no-cache git gcc musl-dev linux-headers elfutils-libelf zlib libstdc++ libgcc
WORKDIR $GOPATH/src/github.com/dswarbrick/ebpf_exporter
COPY Gopkg.lock Gopkg.toml ./
RUN dep ensure --vendor-only
COPY . .
COPY --from=0 /usr/lib64/libbcc.so.0.5.0 /usr/lib/
RUN ln -s /usr/lib/libbcc.so.0.5.0 /usr/lib/libbcc.so.0 && \
    ln -s /usr/lib/libbcc.so.0.5.0 /usr/lib/libbcc.so
COPY --from=0 /usr/include/bcc /usr/include/bcc
COPY --from=0 /usr/lib64/pkgconfig/libbcc.pc /usr/lib64/pkgconfig/
RUN go install -ldflags="-s -w" . 

FROM alpine:edge
RUN apk --no-cache --update add elfutils-libelf zlib libstdc++ libgcc
COPY --from=0 /usr/lib64/libbcc.so.0.5.0 /usr/lib/
RUN ln -s /usr/lib/libbcc.so.0.5.0 /usr/lib/libbcc.so.0 && \
    ln -s /usr/lib/libbcc.so.0.5.0 /usr/lib/libbcc.so
COPY --from=1 /go/bin/ebpf_exporter /
CMD ["/ebpf_exporter"]
