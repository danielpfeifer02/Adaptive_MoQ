EGRESS = egress_t
EGRESS_PATH = egress
INGRESS = ingress_t
INGRESS_PATH = ingress
BPF_EGRESS = ${EGRESS_PATH}/${EGRESS:=_handling}
BPF_INGRESS = ${INGRESS_PATH}/${INGRESS:=_handling}
BPF_EGRESS_C = ${BPF_EGRESS:=.c}
BPF_INGRESS_C = ${BPF_INGRESS:=.c}
BPF_EGRESS_OBJ = ${BPF_EGRESS_C:.c=.o}
BPF_INGRESS_OBJ = ${BPF_INGRESS_C:.c=.o}

BPF_EGRESS_PINNED_PATH := /sys/fs/bpf/$(EGRESS)
BPF_INGRESS_PINNED_PATH := /sys/fs/bpf/$(INGRESS)
EGRESS_NAME := egress_exec
INGRESS_NAME := ingress_exec
DEV := lo

CC = gcc
CFLAGS = -Wall -Wextra -g
LDFLAGS = -lbpf


APC_SET_PATH = manage
APC_SET = $(APC_SET_PATH)/apc_set
APC_SET_C = $(APC_SET:=.c)

ingress: $(BPF_INGRESS_OBJ)
	-bpftool net detach xdpgeneric dev $(DEV)
	rm -f $(BPF_INGRESS_PINNED_PATH)
	
	rm -f /sys/fs/bpf/meta
	rm -f /sys/fs/bpf/payload_mp
#	create a map to store meta data (update value size if content changes)
#   TODO change sizes to correct values
#	bpftool map create /sys/fs/bpf/meta type hash key 16 value 4 entries 16 name meta
#	bpftool map create /sys/fs/bpf/payload_mp type array key 4 value 1 entries 256 name payload_mp
#	bpftool map create /sys/fs/bpf/adaptive_flag type array key 4 value 1 entries 1 name adaptive_flag


	bpftool prog load $(BPF_INGRESS_OBJ) $(BPF_INGRESS_PINNED_PATH)
	bpftool net attach xdpgeneric pinned $(BPF_INGRESS_PINNED_PATH) dev $(DEV)

$(BPF_INGRESS_OBJ): ${INGRESS_PATH}/%.o: ${INGRESS_PATH}/%.c
	clang -S \
		-g \
		-target bpf \
	  -I../libbpf/src\
		-Wall \
		-fno-builtin \
		-O2 -emit-llvm -c -o ${@:.o=.ll} $<
	llc -march=bpf -filetype=obj -O2 -o $@ ${@:.o=.ll}

egress: $(BPF_EGRESS_OBJ)
	tc qdisc add dev ${DEV} clsact
#	tc filter add dev ${DEV} ingress bpf da obj ${BPF_EGRESS_OBJ} sec ingress
	tc filter add dev ${DEV} egress bpf da obj ${BPF_EGRESS_OBJ} sec egress
#	tc filter show dev ${DEV} ingress
	tc filter show dev ${DEV} egress

$(BPF_EGRESS_OBJ): ${EGRESS_PATH}/%.o: ${EGRESS_PATH}/%.c
	clang -S \
		-g \
		-target bpf \
	  -I../libbpf/src\
	  -I../iproute2/include\
		-Wall \
		-Werror \
		-O2 -emit-llvm -c -o ${@:.o=.ll} $<
	llc -march=bpf -filetype=obj -O2 -o $@ ${@:.o=.ll}


manage: $(APC_SET)
	sudo chmod 777 /sys/fs/bpf/adaptive_flag

$(APC_SET): $(APC_SET_C)
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)

clean:
	-bpftool net detach xdpgeneric dev $(DEV)
	sudo rm -f $(BPF_INGRESS_PINNED_PATH)
	sudo rm -f $(BPF_EGRESS_PINNED_PATH)

#	tc filter del dev ${DEV} ingress
	@if [ -e "$(BPF_EGRESS_OBJ)" ]; then \
		echo "Deleting egress TC rules"; \
		tc filter del dev ${DEV} egress; \
		tc qdisc del dev ${DEV} clsact; \
	fi

	rm -f $(BPF_INGRESS_OBJ)
	rm -f ${BPF_INGRESS_OBJ:.o=.ll}
	rm -f $(BPF_EGRESS_OBJ)
	rm -f ${BPF_EGRESS_OBJ:.o=.ll}
	rm -f $(APC_SET)
	rm -f /sys/fs/bpf/adaptive_flag
#	rm -f /sys/fs/bpf/meta
#	rm -f /sys/fs/bpf/payload_mp
