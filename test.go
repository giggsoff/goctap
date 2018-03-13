package main

import (
	"log"

	"github.com/songgao/packets/ethernet"
	"github.com/giggsoff/water"

	"github.com/klauspost/reedsolomon"
	"flag"
	"fmt"
	"os"
	"net"
	"runtime"
	"io"
)

var dataShards = flag.Int("data", 4, "Number of shards to split the data into, must be below 257.")
var parShards = flag.Int("par", 2, "Number of parity shards")
var lipport = flag.String("lipport", "0.0.0.0:10001", "Listening IP:port")
var cipport = flag.String("cipport", "127.0.0.1:10002", "Connection IP:port")
var useRS = flag.Int("users", 0, "Use RS")

func listenUDP(connection *net.UDPConn, quit chan struct{}, intf *water.Interface) {
	buffer := make([]byte, 1600)
	n, remoteAddr, err := 0, new(net.UDPAddr), error(nil)
	for err == nil {
		n, remoteAddr, err = connection.ReadFromUDP(buffer)
		// you might copy out the contents of the packet here, to
		// `var r myapp.Request`, say, and `go handleRequest(r)` (or
		// send it down a channel) to free up the listening
		// goroutine. you do *need* to copy then, though,
		// because you've only made one buffer per listen().
		fmt.Println("from", remoteAddr, "-", n)
		intf.Write(buffer[:n])
	}
	fmt.Println("listener failed - ", err)
	quit <- struct{}{}
}
func listenTAP(intf *water.Interface, connection *net.UDPConn, quit chan struct{}, useRS int, rsEnc reedsolomon.Encoder) {
	var frame ethernet.Frame
	frame.Resize(1550)
	var err = error(nil)
	for err == nil {
		n, err := intf.Read([]byte(frame))
		if n == 0 || (err != nil && err != io.EOF) {
			continue
		}
		log.Printf("n: %d\n", n)
		checkErr(err)
		frame = frame[:n]
		_, err = connection.Write(frame)
		checkErr(err)
		log.Printf("Dst: %s\n", frame.Destination())
		log.Printf("Src: %s\n", frame.Source())
		log.Printf("Ethertype: % x\n", frame.Ethertype())
		log.Printf("Payload: % x\n", frame.Payload())
		if useRS > 0 {
			shards, err := rsEnc.Split(frame.Payload())
			checkErr(err)
			//fmt.Printf("File split into %d data+parity shards with %d bytes/shard.\n", len(shards), len(shards[0]))
			// Encode parity
			err = rsEnc.Encode(shards)
			checkErr(err)
			ok, err := rsEnc.Verify(shards)
			if ok {
				//fmt.Println("No reconstruction needed")
			} else {
				fmt.Println("Verification failed. Reconstructing data")
				err = rsEnc.Reconstruct(shards)
				if err != nil {
					fmt.Println("Reconstruct failed -", err)
					os.Exit(1)
				}
				ok, err = rsEnc.Verify(shards)
				if !ok {
					fmt.Println("Verification failed after reconstruction, data likely corrupted.")
					os.Exit(1)
				}
				checkErr(err)
			}
		}
	}
	quit <- struct{}{}
}
func main() {
	flag.Parse()
	enc, err := reedsolomon.New(*dataShards, *parShards)
	checkErr(err)
	ifce, err := water.New(water.Config{
		DeviceType: water.TAP,
	})
	if err != nil {
		log.Fatal(err)
	}
	LocalAddr, err := net.ResolveUDPAddr("udp4", *lipport)
	checkErr(err)
	ServerAddr, err := net.ResolveUDPAddr("udp4", *cipport)
	checkErr(err)
	Conn, err := net.DialUDP("udp4", LocalAddr, ServerAddr)
	checkErr(err)
	//connection, err := net.ListenUDP("udp", LocalAddr)
	//checkErr(err)
	quitUDP := make(chan struct{})
	fmt.Print(runtime.NumCPU())
	for i := 0; i < 1; i++ {
		go listenUDP(Conn, quitUDP, ifce)
	}
	quitTAP := make(chan struct{})
	for i := 0; i < 1; i++ {
		go listenTAP(ifce, Conn, quitTAP, *useRS, enc)
	}
	<-quitUDP
	<-quitTAP
}

func checkErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(2)
	}
}
