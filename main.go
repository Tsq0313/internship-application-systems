package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"net"
	"os"
	"time"
)

type ICMP struct {
	Type        uint8
	Code        uint8
	Checksum    uint16
	Identifier  uint16
	SequenceNum uint16
	time int64
}

var (
	seqID = 0
)
func Int64FromBytes(bytes []byte) int64 {
	int := binary.BigEndian.Uint64(bytes)
	return int64(int)
}

func CheckSum(data [] byte) uint16 {
	var (
		sum uint32
		countTo int = len(data)
		count int
	)
	for countTo > 1 {
		sum += uint32(data[count]) << 8 + uint32(data[count + 1])
		count += 2
		countTo -= 2
	}

	if countTo > 0 {
		sum += uint32(data[count + 1])
	}
	sum += sum >> 16
	sum = ^sum

	return uint16(sum)
}

func ReceiveOnePing(connection net.Conn, timeout int64, protocolID int) (int, string,
	uint16, int, int64, error) {
	var (
		timeLeft = timeout * 1e9
		err error
		header4 *ipv4.Header
		header6 *ipv6.Header
		icmpInfo *icmp.Message
		sourceAddr net.IP
		TTL int
	)
	for timeLeft > 0 {

		receivePacket := make([] byte, 1024)
		err = connection.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
		if err != nil {
			return  0, "", 0, 0,0, err
		}

		//size, sourceAddr, err := connection.ReadFrom(receivePacket)
		_, err = connection.Read(receivePacket)
		timeReceived := time.Now().UnixNano()
		if err != nil {
			return 0, "", 0, 0,0, err
		}
		// Extract header from IP packet
		if protocolID == 1 {
			header4, err = ipv4.ParseHeader(receivePacket)
			sourceAddr = header4.Src
			TTL = header4.TTL
			icmpInfo, err = icmp.ParseMessage(protocolID, receivePacket[header4.Len: header4.TotalLen])
		} else if protocolID == 58 {
			header6, err = ipv6.ParseHeader(receivePacket)
			sourceAddr = header6.Src
			TTL = header6.HopLimit
			icmpInfo, err = icmp.ParseMessage(protocolID, receivePacket[40: ])
		}
		if err != nil {
			return 0, "", 0, 0, 0, err
		}

		// Extract ICMP message (startTime)
		icmpBody := icmpInfo.Body
		if err != nil {
			return  0, "", 0, 0, 0, err
		}

		length := icmpBody.Len(protocolID)
		data, err := icmpBody.Marshal(protocolID)
		if err != nil {
			return  0, "", 0, 0, 0, err
		}
		timeStart := Int64FromBytes(data[4:])
		seqNum := binary.BigEndian.Uint16(data[2:4])
		RTT := timeReceived - timeStart
		switch icmpInfo.Type {
		case ipv4.ICMPTypeEchoReply:
			return length, sourceAddr.String(), seqNum, TTL, RTT, nil
		case ipv6.ICMPTypeEchoReply:
			return length, sourceAddr.String(), seqNum, TTL, RTT, nil

		default:
			return 0, "", 0, 0,0, errors.New("type does not match")
		}

		timeLeft -= RTT
	}
	return 0, "", 0, 0, 0, nil

}

func SendOnePing(connection net.Conn, ID int) {
	var (
		icmpMessage ICMP
		buffer bytes.Buffer
	)

	// Set ICMP message parameters
	icmpMessage.Type = 8
	icmpMessage.Code = 0
	icmpMessage.Checksum = 0
	icmpMessage.Identifier = uint16(ID)
	icmpMessage.SequenceNum = uint16(seqID)
	icmpMessage.time = time.Now().UnixNano()

	seqID += 1

	// Write message and calculate checksum
	binary.Write(&buffer, binary.BigEndian, icmpMessage)
	icmpMessage.Checksum = CheckSum(buffer.Bytes())
	buffer.Reset()
	binary.Write(&buffer, binary.BigEndian, icmpMessage)

	_, err := connection.Write(buffer.Bytes())
	if err != nil {
		fmt.Println(err.Error())
	}
	return
}

func DoOnePing(destination string, timeout int64, protocolID int) (int, string, uint16, int, int64, error) {
	var (
		connection net.Conn
		err error
	)
	if protocolID == 1 {
		connection, err = net.Dial("ip4:icmp", destination)
		//connection, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	} else if protocolID == 58 {
		connection, err = net.Dial("ip6:ipv6-icmp", destination)
		//connection, err = icmp.ListenPacket("ip6:ipv6-icmp", "::")
	}

	if err != nil {
		return 0, "", 0, 0, 0, err
	}
	defer connection.Close()
	//if protocolID == 1 {
	//	opts := ipv4.NewConn(connection)
	//	err = opts.SetTTL(TTL)
	//} else if protocolID == 58 {
	//	opts := ipv6.NewConn(connection)
	//	err = opts.SetHopLimit(TTL)
	//}
	//if err != nil {
	//	return 0, "", 0, 0, 0, err
	//}
	ID := os.Getpid() & 0xFFFF
	SendOnePing(connection, ID)
	length, sourceAddr, seqNum, TTL, RTT, err := ReceiveOnePing(connection, timeout, protocolID)
	return length, sourceAddr, seqNum, TTL, RTT, err
}

func Ping(hostname string, timeout int64, protocolID int)  {
	var (
		err error
	)

	//if protocolID == 1 {
	//	destination, err = net.ResolveIPAddr("ip4", hostname)
	//} else if protocolID == 58 {
	//	destination, err = net.ResolveIPAddr("ip6", hostname)
	//}

	if err != nil {
		panic(err)
		fmt.Println(err.Error())
		return
	}
	fmt.Println("Pinging " + hostname + ":")

	// Send ping requests to a server every 1 second
	for {
		length, sourceAddr, seqNum, TTL, RTT, err := DoOnePing(hostname, timeout, protocolID)
		if err != nil {
			fmt.Println(err.Error())
		} else {
			rtt := float64(RTT)
			rtt /= 1e6
			printout := fmt.Sprintf("%d bytes from %s: icmp_seq=%d TTL=%d RTT=%.3fms",
				length, sourceAddr, seqNum, TTL, rtt)
			fmt.Println(printout)
		}
		time.Sleep(time.Duration(timeout) * time.Second)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ", os.Args[0], "hostname")
		os.Exit(0)
	}
	var (
		hostname string
		timeout int64
		protocolID int
	)
	hostname = os.Args[1]
	timeout = int64(1)
	protocolID = 1

	Ping (hostname, timeout, protocolID)
}
