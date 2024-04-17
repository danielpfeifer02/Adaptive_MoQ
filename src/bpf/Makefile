TC = tc_t # tc_poc
TC_PATH = tc
BPF_TC = ${TC_PATH}/${TC:=_handling}
BPF_TC_C = ${BPF_TC:=.c}
BPF_TC_OBJ = ${BPF_TC_C:.c=.o}

BPF_TC_PINNED_PATH := /sys/fs/bpf/$(TC)

TC_IN_TO_EG = tc_poc
TC_IN_TO_EG_PATH = tc
BPF_TC_IN_TO_EG = ${TC_IN_TO_EG_PATH}/${TC_IN_TO_EG:=_handling}
BPF_TC_IN_TO_EG_C = ${BPF_TC_IN_TO_EG:=.c}
BPF_TC_IN_TO_EG_OBJ = ${BPF_TC_IN_TO_EG_C:.c=.o}

BPF_TC_IN_TO_EG_PINNED_PATH := /sys/fs/bpf/$(TC_IN_TO_EG)

DEV_IN := veth1
DEV_EG := veth2

# TODO: other way to export PATH
export PATH := $(PATH):/usr/local/go/bin

PRIO_DROP_PATH := ../go/examples/priority_drop
INGRESS_TO_EGRESS_PATH := ../go/examples/ingress_to_egress

.PHONY: clean all tc_main tc_poc

all: $(BPF_TC_OBJ) tc_main
	go build -o ${PRIO_DROP_PATH}/build/main ${PRIO_DROP_PATH}/*.go

poc: $(BPF_TC_IN_TO_EG_OBJ) tc_poc
	@if [ ! -e "/sys/kernel/tracing/trace_pipe" ]; then \
		echo "Mounting tracefs"; \
		sudo mount -t tracefs tracefs /sys/kernel/tracing; \
	fi
	go build -o ${INGRESS_TO_EGRESS_PATH}/build ${INGRESS_TO_EGRESS_PATH}/main.go

tc_main: $(BPF_TC_OBJ)
	tc qdisc add dev ${DEV_IN} clsact
	tc qdisc add dev ${DEV_EG} clsact
	tc filter add dev ${DEV_IN} ingress bpf da obj ${BPF_TC_OBJ} sec ingress
	tc filter add dev ${DEV_EG} egress bpf da obj ${BPF_TC_OBJ} sec egress
	tc filter add dev ${DEV_EG} ingress bpf da obj ${BPF_TC_OBJ} sec ingress_from_client
	tc filter show dev ${DEV_IN} ingress
	tc filter show dev ${DEV_EG} egress

tc_poc: $(BPF_TC_IN_TO_EG_OBJ)
	tc qdisc add dev ${DEV_IN} clsact
	tc qdisc add dev ${DEV_EG} clsact
	tc filter add dev ${DEV_IN} ingress bpf da obj ${BPF_TC_IN_TO_EG_OBJ} sec ingress
	tc filter add dev ${DEV_EG} egress bpf da obj ${BPF_TC_IN_TO_EG_OBJ} sec egress
	tc filter show dev ${DEV_IN} ingress
	tc filter show dev ${DEV_EG} egress

# TODO add -Werror flag 
$(BPF_TC_OBJ): ${TC_PATH}/%.o: ${TC_PATH}/%.c
	clang -S \
		-g \
		-target bpf \
	  -I../../libbpf/src\
	  -I../../iproute2/include\
		-Wall \
		-O2 -emit-llvm -c -o ${@:.o=.ll} $<
	llc -march=bpf -filetype=obj -O2 -o $@ ${@:.o=.ll}

# TODO add -Werror flag 
$(BPF_TC_IN_TO_EG_OBJ): ${TC_IN_TO_EG_PATH}/%.o: ${TC_IN_TO_EG_PATH}/%.c
	clang -S \
		-g \
		-target bpf \
	  -I../../libbpf/src\
	  -I../../iproute2/include\
		-Wall \
		-O2 -emit-llvm -c -o ${@:.o=.ll} $<
	llc -march=bpf -filetype=obj -O2 -o $@ ${@:.o=.ll}	

clean:
	sudo rm -f $(BPF_TC_PINNED_PATH)

	@if [ -e "$(BPF_TC_OBJ)" ]; then \
		echo "Deleting TC filters and maps"; \
		tc filter del dev ${DEV_EG} egress; \
		tc filter del dev ${DEV_IN} ingress; \
		tc qdisc del dev ${DEV_EG} clsact; \
		tc qdisc del dev ${DEV_IN} clsact; \
		rm -f $(BPF_TC_OBJ); \
		rm -f ${BPF_TC_OBJ:.o=.ll}; \
		rm -f /sys/fs/bpf/tc/globals/id_counter; \
		rm -f /sys/fs/bpf/tc/globals/client_data; \
		rm -f /sys/fs/bpf/tc/globals/number_of_clients; \
		rm -f /sys/fs/bpf/tc/globals/client_id; \
		rm -f /sys/fs/bpf/tc/globals/packet_counter; \
		rm -f /sys/fs/bpf/tc/globals/connection_established; \
		rm -f /sys/fs/bpf/tc/globals/client_pn; \
		rm -f /sys/fs/bpf/tc/globals/connection_current_pn; \
		rm -f /sys/fs/bpf/tc/globals/connection_pn_translation; \
		rm -f /sys/fs/bpf/tc/globals/client_stream_offset; \
	fi

	sudo rm -f $(BPF_TC_IN_TO_EG_PINNED_PATH)

	@if [ -e "$(BPF_TC_IN_TO_EG_OBJ)" ]; then \
		echo "Deleting TC filters"; \
		tc filter del dev ${DEV_EG} egress; \
		tc filter del dev ${DEV_IN} ingress; \
		tc qdisc del dev ${DEV_EG} clsact; \
		tc qdisc del dev ${DEV_IN} clsact; \
		rm -f $(BPF_TC_IN_TO_EG_OBJ); \
		rm -f ${BPF_TC_IN_TO_EG_OBJ:.o=.ll}; \
	fi

	rm -f ${PRIO_DROP_PATH}/build/main
	rm -f ${INGRESS_TO_EGRESS_PATH}/build/main