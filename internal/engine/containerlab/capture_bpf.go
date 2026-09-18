package containerlab

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -target bpf -cflags "-I/usr/include/x86_64-linux-gnu" bpfcap ../../../bpf/capture.bpf.c
