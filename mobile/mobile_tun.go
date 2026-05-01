// This file contains TUN bridge implementation integrated into the mobile package
package mobile

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	// DNS server that will be advertised to Android
	TunDNSServer = "172.19.0.2"

	// TUN interface addresses
	TunClientIP = "172.19.0.1"
	TunRouterIP = "172.19.0.2"

	// Fake DNS range
	FakeDNSStart = "198.18.0.0"
	FakeDNSEnd   = "198.19.255.255"

	// Buffer sizes
	tunMTU       = 1500
	relayBufSize = 8192
)

var (
	tunBytesUp   atomic.Int64
	tunBytesDown atomic.Int64
)

// tunBridge manages the TUN interface and DNS interception
type tunBridge struct {
	tunFd      int
	socksAddr  string
	dnsMap     *dnsMapper
	tcpHandler *tcpHandler
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	protectFn  func(fd int) bool
}

// dnsMapper handles fake DNS mapping
type dnsMapper struct {
	mu           sync.RWMutex
	hostnameToIP map[string]string
	ipToHostname map[string]string
	counter      uint32
}

func newDNSMapper() *dnsMapper {
	return &dnsMapper{
		hostnameToIP: make(map[string]string),
		ipToHostname: make(map[string]string),
		counter:      1,
	}
}

func (d *dnsMapper) getFakeIP(hostname string) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if ip, ok := d.hostnameToIP[hostname]; ok {
		return ip
	}

	// Generate fake IP: 198.18.x.x
	counter := atomic.AddUint32(&d.counter, 1)
	if counter > 65535 {
		atomic.StoreUint32(&d.counter, 1)
		counter = 1
	}

	octet3 := byte(counter >> 8)
	octet4 := byte(counter & 0xFF)
	fakeIP := fmt.Sprintf("198.18.%d.%d", octet3, octet4)

	d.hostnameToIP[hostname] = fakeIP
	d.ipToHostname[fakeIP] = hostname

	log.Printf("[TUN-DNS] Mapped %s -> %s", hostname, fakeIP)
	return fakeIP
}

func (d *dnsMapper) getHostname(fakeIP string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	hostname, ok := d.ipToHostname[fakeIP]
	return hostname, ok
}

// newTunBridge creates a new TUN bridge
func newTunBridge(tunFd int, socksAddr string, protectFn func(int) bool) *tunBridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &tunBridge{
		tunFd:      tunFd,
		socksAddr:  socksAddr,
		dnsMap:     newDNSMapper(),
		tcpHandler: nil,
		ctx:        ctx,
		cancel:     cancel,
		protectFn:  protectFn,
	}
}

// start starts the TUN bridge
func (b *tunBridge) start() error {
	log.Printf("[TUN-BRIDGE] Starting bridge: tunFd=%d socksAddr=%s", b.tunFd, b.socksAddr)

	tunBytesUp.Store(0)
	tunBytesDown.Store(0)

	// Create TCP handler
	b.tcpHandler = newTCPHandler(b, b.socksAddr)
	log.Printf("[TUN-BRIDGE] TCP handler created")

	// Start packet processing
	b.wg.Add(1)
	go b.processPackets()

	return nil
}

// stop stops the TUN bridge
func (b *tunBridge) stop() {
	log.Printf("[TUN-BRIDGE] Stopping bridge")
	b.cancel()

	if b.tcpHandler != nil {
		b.tcpHandler.close()
	}

	b.wg.Wait()
}

// getBandwidth returns upload and download bytes
func (b *tunBridge) getBandwidth() (up, down int64) {
	return tunBytesUp.Load(), tunBytesDown.Load()
}

// processPackets reads packets from TUN and handles them
func (b *tunBridge) processPackets() {
	defer b.wg.Done()

	tunFile := &tunFile{fd: b.tunFd}
	buf := make([]byte, tunMTU+4) // +4 for packet info

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		// Read packet from TUN
		n, err := tunFile.Read(buf)
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			log.Printf("[TUN-BRIDGE] Read error: %v", err)
			continue
		}

		if n < 20 { // Minimum IP header size
			continue
		}

		tunBytesUp.Add(int64(n))

		// Parse IP packet
		packet := buf[:n]
		b.handlePacket(packet)
	}
}

// handlePacket processes a single IP packet
func (b *tunBridge) handlePacket(packet []byte) {
	if len(packet) < 20 {
		return
	}

	// Check IP version
	version := packet[0] >> 4
	if version != 4 {
		// IPv6 not supported yet
		return
	}

	// Get protocol
	protocol := packet[9]

	// Get source and destination IPs
	srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
	dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19])

	// Handle UDP (DNS)
	if protocol == 17 { // UDP
		b.handleUDP(packet, srcIP, dstIP)
		return
	}

	// Handle TCP
	if protocol == 6 { // TCP
		b.tcpHandler.handleTCP(packet, srcIP, dstIP)
		return
	}
}

// handleUDP handles UDP packets (mainly DNS)
func (b *tunBridge) handleUDP(packet []byte, srcIP, dstIP net.IP) {
	if len(packet) < 28 { // IP header (20) + UDP header (8)
		return
	}

	// Parse UDP header
	srcPort := binary.BigEndian.Uint16(packet[20:22])
	dstPort := binary.BigEndian.Uint16(packet[22:24])

	// Check if it's DNS (port 53)
	if dstPort == 53 {
		b.handleDNS(packet, srcIP, srcPort)
		return
	}

	// Other UDP traffic - not supported yet
	log.Printf("[TUN-BRIDGE] UDP packet: %s:%d -> %s:%d (not DNS, dropping)",
		srcIP, srcPort, dstIP, dstPort)
}

// handleDNS handles DNS queries
func (b *tunBridge) handleDNS(packet []byte, srcIP net.IP, srcPort uint16) {
	if len(packet) < 28 {
		return
	}

	// Extract DNS query
	dnsQuery := packet[28:]

	// Parse DNS query to get hostname
	hostname := b.parseDNSQuery(dnsQuery)
	if hostname == "" {
		return
	}

	// Get fake IP for hostname
	fakeIP := b.dnsMap.getFakeIP(hostname)

	// Build DNS response
	response := b.buildDNSResponse(dnsQuery, fakeIP)
	if response == nil {
		return
	}

	// Send response back through TUN
	b.sendDNSResponse(packet, srcIP, srcPort, response)
}

// parseDNSQuery extracts hostname from DNS query
func (b *tunBridge) parseDNSQuery(query []byte) string {
	if len(query) < 12 {
		return ""
	}

	pos := 12 // Skip DNS header
	labels := []string{}

	for pos < len(query) {
		length := int(query[pos])
		if length == 0 {
			break
		}
		if length > 63 || pos+1+length > len(query) {
			return ""
		}

		pos++
		label := string(query[pos : pos+length])
		labels = append(labels, label)
		pos += length
	}

	if len(labels) == 0 {
		return ""
	}

	hostname := ""
	for i, label := range labels {
		if i > 0 {
			hostname += "."
		}
		hostname += label
	}

	return hostname
}

// buildDNSResponse builds a DNS response for a hostname
func (b *tunBridge) buildDNSResponse(query []byte, ip string) []byte {
	if len(query) < 12 {
		return nil
	}

	// Build response
	response := make([]byte, len(query)+16)
	copy(response, query)

	// Set response flags: QR=1, RD=1
	response[2] = 0x84
	response[3] = 0x00

	// Set answer count
	response[6] = 0x00
	response[7] = 0x01

	// Append answer section
	answerPos := len(query)
	// Pointer to question name
	response[answerPos] = 0xC0
	response[answerPos+1] = 0x0C

	// Type A, Class IN
	response[answerPos+2] = 0x00
	response[answerPos+3] = 0x01
	response[answerPos+4] = 0x00
	response[answerPos+5] = 0x01

	// TTL (60 seconds)
	response[answerPos+6] = 0x00
	response[answerPos+7] = 0x00
	response[answerPos+8] = 0x00
	response[answerPos+9] = 0x3C

	// RDLEN
	response[answerPos+10] = 0x00
	response[answerPos+11] = 0x04

	// IP address
	parts := net.ParseIP(ip)
	if parts == nil {
		return nil
	}
	copy(response[answerPos+12:answerPos+16], parts.To4())

	return response[:answerPos+16]
}

// sendDNSResponse sends a DNS response back to the client
func (b *tunBridge) sendDNSResponse(originalPacket []byte, clientIP net.IP, clientPort uint16, dnsResponse []byte) {
	if len(originalPacket) < 20 {
		return
	}

	// Build response IP packet
	responsePacket := make([]byte, 20+8+len(dnsResponse))

	// Copy and swap IPs/ports from original packet
	copy(responsePacket[:20], originalPacket[:20])

	// Swap source and destination IPs
	for i := 0; i < 4; i++ {
		responsePacket[12+i], responsePacket[16+i] = responsePacket[16+i], responsePacket[12+i]
	}

	// UDP header
	responsePacket[20] = byte(53 >> 8)      // Source port DNS high byte
	responsePacket[21] = byte(53)            // Source port DNS low byte
	responsePacket[22] = byte(clientPort >> 8)
	responsePacket[23] = byte(clientPort)

	// UDP length and checksum (simplified, checksum set to 0)
	udpLen := 8 + len(dnsResponse)
	responsePacket[24] = byte(udpLen >> 8)
	responsePacket[25] = byte(udpLen)
	responsePacket[26] = 0
	responsePacket[27] = 0

	// Copy DNS response
	copy(responsePacket[28:], dnsResponse)

	// Write back to TUN
	tunFile := &tunFile{fd: b.tunFd}
	tunFile.Write(responsePacket)

	tunBytesDown.Add(int64(len(responsePacket)))
}

// tunFile wraps TUN file operations
type tunFile struct {
	fd int
}

// Read reads from TUN file descriptor
func (t *tunFile) Read(p []byte) (n int, err error) {
	n, err = syscall.Read(t.fd, p)
	return
}

// Write writes to TUN file descriptor
func (t *tunFile) Write(p []byte) (n int, err error) {
	n, err = syscall.Write(t.fd, p)
	return
}

// tcpHandler manages TCP connections through SOCKS5
type tcpHandler struct {
	bridge    *tunBridge
	mu        sync.RWMutex
	sessions  map[string]*tcpSession
	socksAddr string
}

// tcpSession represents a TCP connection
type tcpSession struct {
	id         string
	srcIP      net.IP
	srcPort    uint16
	dstIP      net.IP
	dstPort    uint16
	socksConn  net.Conn
	state      tcpState
	seqNum     uint32
	ackNum     uint32
	lastActive time.Time
	mu         sync.Mutex
}

type tcpState int

const (
	tcpStateClosed tcpState = iota
	tcpStateSynSent
	tcpStateSynReceived
	tcpStateEstablished
	tcpStateFinWait1
	tcpStateFinWait2
	tcpStateClosing
	tcpStateTimeWait
	tcpStateCloseWait
	tcpStateLastAck
)

// newTCPHandler creates a new TCP handler
func newTCPHandler(bridge *tunBridge, socksAddr string) *tcpHandler {
	return &tcpHandler{
		bridge:    bridge,
		sessions:  make(map[string]*tcpSession),
		socksAddr: socksAddr,
	}
}

// handleTCP handles TCP packets
func (h *tcpHandler) handleTCP(packet []byte, srcIP, dstIP net.IP) {
	if len(packet) < 40 { // IP header (20) + TCP header (20)
		return
	}

	srcPort := binary.BigEndian.Uint16(packet[20:22])
	dstPort := binary.BigEndian.Uint16(packet[22:24])

	log.Printf("[TCP] Packet: %s:%d -> %s:%d", srcIP, srcPort, dstIP, dstPort)

	// Extract hostname from dstIP if it's a fake DNS IP
	hostname := ""
	if dstIP.To4() != nil {
		ipStr := dstIP.String()
		if h, ok := h.bridge.dnsMap.getHostname(ipStr); ok {
			hostname = h
		}
	}

	if hostname != "" {
		log.Printf("[TCP] Redirecting to real hostname: %s", hostname)
		// TODO: Implement SOCKS5 tunnel with real hostname
	}
}

// close closes the TCP handler
func (h *tcpHandler) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, session := range h.sessions {
		if session.socksConn != nil {
			_ = session.socksConn.Close()
		}
	}
	h.sessions = make(map[string]*tcpSession)
}

// Protect protects a socket file descriptor from VPN routing
func Protect(fd int, protectFn func(int) bool) bool {
	if protectFn != nil {
		return protectFn(fd)
	}
	return false
}

// SetNonblock sets a file descriptor to non-blocking mode
func SetNonblock(fd int, nonblocking bool) error {
	return syscall.SetNonblock(fd, nonblocking)
}

// GetSockOpt gets socket option
func GetSockOpt(fd, level, opt int) (int, error) {
	var value int
	vallen := uint32(unsafe.Sizeof(value))
	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(fd),
		uintptr(level),
		uintptr(opt),
		uintptr(unsafe.Pointer(&value)),
		uintptr(unsafe.Pointer(&vallen)),
		0,
	)
	if errno != 0 {
		return 0, errno
	}
	return value, nil
}
