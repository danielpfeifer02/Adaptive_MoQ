package common

// TODO: add everything here that is used for the video example as well as for the performance analysis

// TODO: change all calls from the examples from bpf_handler.go to here

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/danielpfeifer02/quic-go-prio-packs"
	"github.com/danielpfeifer02/quic-go-prio-packs/packet_setting"
)

// This function will clear all BPF maps once the relay is started.
// This relies on an external C program since there exist wrappers
// to iterate over keys of a map.
// Ideally this would also be done in Go.
// TODO: not the most elegant way to clear the BPF maps
func ClearBPFMaps() {

	// TODO: do with ebpf library

	paths := []string{
		"client_data",
		"client_id",
		"id_counter",
		"number_of_clients",
		"client_pn",
		"connection_current_pn",
		"connection_pn_translation",
		"connection_unistream_id_translation",
		"client_stream_offset"}
	map_location := "/sys/fs/bpf/tc/globals/"

	for _, path := range paths {
		cmd := exec.Command("../../utils/build/clear_bpf_map", map_location+path)
		stdout, err := cmd.Output()
		if err != nil {
			fmt.Println(string(stdout))
			panic(err)
		}
		fmt.Println(string(stdout))
	}
}

// Create a global list of connection ids (i.e. <20 byte long byte arrays)
// when a new connection is initiated, add the connection id to the list
// when a connection is retired, remove the connection id from the list

// This map stores the connection ids given a key that consists of the
// IP address and port of a connection.
var connection_ids map[[6]byte][][]byte

// We need a lock for the global list since the
// underlying connection management is concurrent.
var mutex = &sync.Mutex{}

// This function is called from within the underlying QUIC implementation
// when a new connection is initiated. It will then be added to the global
// connection_id list.
func InitConnectionId(id []byte, l uint8, conn packet_setting.QuicConnection) {

	qconn := conn.(quic.Connection)

	if qconn.RemoteAddr().String() == packet_setting.SERVER_ADDR {
		// We do only add connection ids for client connections.
		return
	}
	debugPrint("INIT")
	debugPrint("Initialize connection id for connection:", qconn.RemoteAddr().String())

	key := getConnectionIDsKey(qconn)

	// If the key does not exist, create new list.
	mutex.Lock()
	if connection_ids == nil {
		connection_ids = make(map[[6]byte][][]byte)
	}
	if _, ok := connection_ids[key]; !ok {
		connection_ids[key] = make([][]byte, 0)
	}
	mutex.Unlock()

	connection_ids[key] = append(connection_ids[key], id)
}

// This function is called when a connection is retired.
// The connection id is removed from the global list.
func RetireConnectionId(id []byte, l uint8, conn packet_setting.QuicConnection) {

	if len(id) == 0 {
		fmt.Println("Empty connection id???")
		return
	}

	qconn := conn.(quic.Connection)

	if qconn.RemoteAddr().String() == packet_setting.SERVER_ADDR {
		// We only consider connection ids for client connections.
		return
	}
	debugPrint("RETIRE")
	debugPrint("Retire connection id for connection:", qconn.RemoteAddr().String())

	retired_priority := id[0]

	key := getConnectionIDsKey(qconn)
	for i, v := range connection_ids[key] {
		if string(v) == string(id) {
			connection_ids[key] = append(connection_ids[key][:i], connection_ids[key][i+1:]...)
			break
		}
	}

	go func(key [6]byte) {

		// It might be the case that retirements happen in the same packet as initiations
		// and that for a brief time there are no connection ids left.
		// If that is the case just wait until there are connection ids again.
		// If nothing happens after 100 iterations (i.e. 1 second), panic.
		for i := 0; i < 100; i++ {
			if len(connection_ids[key]) > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if len(connection_ids[key]) == 0 {
			panic("No connection ids left")
		}

		qconn := conn.(quic.Connection)

		// TODO: is this correct?
		// TODO: this function does not seem to be called
		// TODO: in the example.
		updated := false
		for _, v := range connection_ids[key] {
			if v[0] == retired_priority {
				SetBPFMapConnectionID(qconn, v)
				updated = true
				break
			}
		}
		if !updated {
			panic("No connection id with the retired priority found!")
		}

		debugPrint("Successfully retired connection id")
	}(key)
}

// This function is called from within the underlying QUIC implementation
// when a connection id is updated.
// It sets the current connection id to the provided one.
func UpdateConnectionId(id []byte, l uint8, conn packet_setting.QuicConnection) {

	qconn := conn.(quic.Connection)

	if qconn.RemoteAddr().String() == packet_setting.SERVER_ADDR {
		// We only consider connection ids for client connections.
		return
	}

	debugPrint("UPDATE")
	SetBPFMapConnectionID(qconn, id)
}

// TODO: mabye split up into a "get client data" function
func SetBPFMapConnectionID(qconn quic.Connection, v []byte) {
	ipaddr, port := GetIPAndPort(qconn, true)
	ipaddr_key := swapEndianness32(ipToInt32(ipaddr))
	port_key := swapEndianness16(port)

	key := client_key_struct{
		Ipaddr:  ipaddr_key,
		Port:    port_key,
		Padding: [2]uint8{0, 0},
	}
	id := &id_struct{}

	// This should not occur since the function pointers should only be set if
	// bpf is enabled.
	// Still to make sure that the program does not panic, we check if bpf is enabled.
	if !true {
		fmt.Println("BPF not enabled. Cannot access maps.")
		return
	}

	fmt.Println("ipaddr", ipaddr, "port", port)

	err := Client_id.Lookup(key, id)
	if err != nil {
		fmt.Println("Error looking up client_id")
		panic(err)
	}

	client_info := &client_data_struct{}
	err = Client_data.Lookup(id, client_info)
	if err != nil {
		fmt.Println("Error looking up client_data")
		panic(err)
	}
	copy(client_info.ConnectionID[:], v)
	err = Client_data.Update(id, client_info, ebpf.UpdateAny)
	if err != nil {
		fmt.Println("Error updating client_data")
		panic(err)
	}

	debugPrint("Successfully updated client_data for retired connection id")
	debugPrint("Priority drop limit of stream is", client_info.PriorityDropLimit)
}

// This function is called from within the underlying QUIC implementation
// and is used when an ack packet number is re-translated (since the
// relay userspace only gets ACKs for packet numbers which have been changed
// by the bpf program).
func TranslateAckPacketNumber(pn int64, conn packet_setting.QuicConnection) (int64, error) {

	qconn := conn.(quic.Connection)

	if qconn.RemoteAddr().String() == packet_setting.SERVER_ADDR {
		// We only consider connection ids for client connections.
		return pn, nil
	}
	debugPrint("TRANSLATE", pn)
	debugPrint("Translated packet number", qconn.RemoteAddr().String())

	ipaddr, port := GetIPAndPort(qconn, true)
	client_key := client_key_struct{
		Ipaddr:  swapEndianness32(ipToInt32(ipaddr)),
		Port:    swapEndianness16(uint16(port)),
		Padding: [2]uint8{0, 0},
	}
	key := client_pn_map_key{
		Key: client_key,
		Pn:  uint32(pn),
	}

	val := &connnection_pn_stuct{}
	err := Connection_pn_translation.Lookup(key, val)
	if err != nil {
		debugPrint("No entry for ", pn)
		return 0, fmt.Errorf("no entry for %d", pn)
	}

	debugPrint(pn, "->", val.Pn)

	translated_pn := int64(val.Pn)
	debugPrint(translated_pn)
	return translated_pn, nil
}

// This function is necessary to keep the bpf map from overflowing with
// too many packet number translations.
// The function is called from within the underlying QUIC implementation
// and deletes the translation for a packet number once it has been seen
// by the relay userspace (where it will be cached somewhere else).
// To check the number of mappings inside of a bpf map you can use the
// following command:
// bpftool map dump name connection_pn_t -j | jq ". | length"
func DeleteAckPacketNumberTranslation(pn int64, conn packet_setting.QuicConnection) {

	qconn := conn.(quic.Connection)

	if qconn.RemoteAddr().String() == packet_setting.SERVER_ADDR {
		// We only consider connection ids for client connections.
		return
	}
	debugPrint("DELETE", pn)
	debugPrint("Deleted translation for packet from", qconn.RemoteAddr().String())

	ipaddr, port := GetIPAndPort(qconn, true)
	client_key := client_key_struct{
		Ipaddr:  swapEndianness32(ipToInt32(ipaddr)),
		Port:    swapEndianness16(uint16(port)),
		Padding: [2]uint8{0, 0},
	}
	key := client_pn_map_key{
		Key: client_key,
		Pn:  uint32(pn),
	}

	err := Connection_pn_translation.Delete(key)
	if err != nil {
		return
	}

	debugPrint("Successfully deleted translation")
}

func GetLargestSentPacketNumber(conn packet_setting.QuicConnection) int64 {

	qconn := conn.(quic.Connection)

	ipaddr, port := GetIPAndPort(qconn, true)
	ipaddr_key := swapEndianness32(ipToInt32(ipaddr))
	port_key := swapEndianness16(port)

	key := client_key_struct{
		Ipaddr:  ipaddr_key,
		Port:    port_key,
		Padding: [2]uint8{0, 0},
	}

	current_pn := &connnection_pn_stuct{}
	err := Connection_current_pn.Lookup(key, current_pn)
	if err != nil {
		fmt.Println("Error looking up connection_current_pn")
		panic(err)
	}

	return int64(current_pn.Pn - 1)
}

// This function is used for registering the packets that have been sent by the
// BPF program.
func RegisterBPFPacket(conn quic.Connection) {

	go func() {

		ipaddr, port := GetIPAndPort(conn, true)
		key := client_key_struct{
			Ipaddr:  swapEndianness32(ipToInt32(ipaddr)),
			Port:    swapEndianness16(port),
			Padding: [2]uint8{0, 0},
		}

		current_pn := &connnection_pn_stuct{}
		for {

			err := Connection_current_pn.Lookup(key, current_pn)
			if err == nil {
				break
			}
			conn.SetHighestSent(int64(current_pn.Pn) - 1)

		}
	}()

	max_register_queue_size := 1 << 11 // 2048
	val := &packet_register_struct{}
	current_index := index_key_struct{
		Index: 0,
	}

	fmt.Println("Start registering packets...")

	for {

		// Check if there are packets to register
		err := Packets_to_register.Lookup(current_index, val)
		if err == nil && val.Valid == 1 { // TODO: why not valid?

			// fmt.Println("Register packet number", val.PacketNumber, "at index", current_index.Index, "at time", time.Now().UnixNano())

			go func(val packet_register_struct, idx index_key_struct, mp *ebpf.Map) { // TODO: speed up when using goroutines?

				var server_pack packet_setting.RetransmissionPacketContainer
				for {
					server_pack = RetreiveServerPacket(int64(val.ServerPN))
					if server_pack.Valid {
						break
					}
					time.Sleep(1 * time.Millisecond) // TODO: optimal?
				}
				if len(server_pack.RawData) == 0 {
					panic("No server packet found")
				}

				// if server_pack.Length != int64(val.Length) { // TODO: useful check?
				// 	fmt.Println(server_pack.Length, val.Length)
				// 	panic("Lengths do not match")
				// }

				packet := packet_setting.PacketRegisterContainerBPF{
					PacketNumber: int64(val.PacketNumber),
					SentTime:     int64(val.SentTime),
					Length:       int64(val.Length), // TODO: length needed if its in server_pack?

					RawData: server_pack.RawData,

					// These two will be set in the wrapper of the quic connection.
					Frames:       nil,
					StreamFrames: nil,
				}

				conn.RegisterBPFPacket(packet)

				// Set valid to 0 to indicate that the packet has been registered
				val.Valid = 0
				err = mp.Update(idx, val, ebpf.UpdateAny)
				if err != nil {
					fmt.Println("Error updating buffer map")
					panic(err)
				}

			}(*val, current_index, Packets_to_register) // this pass by copy is necessary since the goroutine might be executed after the next iteration

			current_index.Index = uint32((current_index.Index + 1) % uint32(max_register_queue_size))
		}

	}
}

func SetConnectionEstablished(ip net.IP, port uint16) error {

	key := client_key_struct{
		Ipaddr:  swapEndianness32(ipToInt32(ip)),
		Port:    swapEndianness16(uint16(port)),
		Padding: [2]uint8{0, 0},
	}

	est := &established_val_struct{
		Established: uint8(1),
	}

	err := Connection_established.Update(key, est, ebpf.UpdateAny)
	if err != nil {
		fmt.Println("Error updating established", err)
		return err
	}

	fmt.Println("Connection established for", ip, ":", port)
	return nil
}
