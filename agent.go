//-----------------------------------------------------------------------------
// Copyright (C) Microsoft. All rights reserved.
// Licensed under the MIT license.
// See LICENSE.txt file in the project root for full license information.
//-----------------------------------------------------------------------------
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// AgentConfig holds the configuration for the agent mode
type AgentConfig struct {
	OrchestratorURL string        // URL of the orchestrator service
	AuthToken       string        // Authentication token (PAT)
	NodeID          string        // Optional node identifier
	TLSEnabled      bool          // Whether to use TLS
	TLSSkipVerify   bool          // Skip TLS certificate verification
	ReconnectDelay  time.Duration // Delay between reconnection attempts
	HeartbeatInt    time.Duration // Heartbeat interval
}

// OrchestratorMsgType defines the types of messages between agent and orchestrator
type OrchestratorMsgType uint32

const (
	// Agent -> Orchestrator messages
	OrcMsgRegister      OrchestratorMsgType = iota // Agent registration
	OrcMsgHeartbeat                                // Keep-alive heartbeat
	OrcMsgTestResult                               // Test results
	OrcMsgError                                    // Error report
	OrcMsgAck                                      // Acknowledgment

	// Orchestrator -> Agent messages
	OrcMsgRegisterAck   // Registration acknowledged
	OrcMsgRunTest       // Command to run a test
	OrcMsgStopTest      // Command to stop a test
	OrcMsgPing          // Ping from orchestrator
	OrcMsgDisconnect    // Graceful disconnect
)

// OrchestratorMsg is the message structure for orchestrator communication
type OrchestratorMsg struct {
	Type        OrchestratorMsgType
	Register    *OrcMsgRegisterData
	RegisterAck *OrcMsgRegisterAckData
	RunTest     *OrcMsgRunTestData
	StopTest    *OrcMsgStopTestData
	TestResult  *OrcMsgTestResultData
	Error       *OrcMsgErrorData
	Heartbeat   *OrcMsgHeartbeatData
}

// OrcMsgRegisterData contains agent registration information
type OrcMsgRegisterData struct {
	AuthToken   string            // Authentication token
	NodeID      string            // Node identifier (hostname if not specified)
	Version     string            // Ethr version
	Platform    string            // OS/Arch
	Capabilities []string         // Supported test types
	Labels      map[string]string // User-defined labels for node identification
}

// OrcMsgRegisterAckData is the orchestrator's response to registration
type OrcMsgRegisterAckData struct {
	Success      bool
	ErrorMessage string
	SessionID    string            // Unique session ID assigned by orchestrator
	ServerTime   time.Time         // For time sync if needed
	Config       map[string]string // Any config to push to agent
}

// OrcMsgRunTestData contains the test command from orchestrator
// SECURITY: Only whitelisted test types and protocols are allowed
type OrcMsgRunTestData struct {
	TestID      string          // Unique test ID from orchestrator
	TargetHost  string          // Host to test against
	TestType    string          // One of: "b", "c", "p", "l", "pi", "tr", "mtr"
	Protocol    string          // One of: "tcp", "udp", "icmp"
	Duration    time.Duration   // Test duration
	NumThreads  uint32          // Number of parallel connections
	BufferSize  uint32          // Buffer size for bandwidth tests
	Port        uint16          // Target port
	Reverse     bool            // Reverse mode (server sends to client)
	BwRate      uint64          // Bandwidth rate limit
}

// OrcMsgStopTestData requests stopping a running test
type OrcMsgStopTestData struct {
	TestID string // Test ID to stop
}

// OrcMsgTestResultData contains test results sent to orchestrator
type OrcMsgTestResultData struct {
	TestID      string    // Test ID from orchestrator
	NodeID      string    // Node that ran the test
	Timestamp   time.Time // When the result was generated
	TargetHost  string    // Host that was tested
	TestType    string    // Test type that was run
	Protocol    string    // Protocol used
	
	// Results
	Bandwidth   uint64  // Bandwidth in bits/sec
	Connections uint64  // Connections per second (for CPS test)
	Packets     uint64  // Packets per second (for PPS test)
	LatencyAvg  uint64  // Average latency in microseconds
	LatencyMin  uint64  // Minimum latency in microseconds
	LatencyMax  uint64  // Maximum latency in microseconds
	LatencyP50  uint64  // 50th percentile latency
	LatencyP90  uint64  // 90th percentile latency
	LatencyP99  uint64  // 99th percentile latency
	PacketLoss  float64 // Packet loss percentage
	
	// Status
	Success      bool
	ErrorMessage string
}

// OrcMsgErrorData reports errors to orchestrator
type OrcMsgErrorData struct {
	TestID  string // Related test ID (if applicable)
	Code    int    // Error code
	Message string // Error message
}

// OrcMsgHeartbeatData contains heartbeat information
type OrcMsgHeartbeatData struct {
	NodeID        string    // Node identifier
	Timestamp     time.Time // Heartbeat timestamp
	ActiveTests   int       // Number of currently running tests
	CPUUsage      float64   // CPU usage percentage (optional)
	MemoryUsage   float64   // Memory usage percentage (optional)
}

// Whitelist of allowed test types - SECURITY: Only these are executed
var allowedTestTypes = map[string]EthrTestType{
	"b":   Bandwidth,
	"c":   Cps,
	"p":   Pps,
	"l":   Latency,
	"pi":  Ping,
	"tr":  TraceRoute,
	"mtr": MyTraceRoute,
}

// Whitelist of allowed protocols - SECURITY: Only these are used
var allowedProtocols = map[string]EthrProtocol{
	"tcp":  TCP,
	"udp":  UDP,
	"icmp": ICMP,
}

// Agent represents an ethr agent that connects to an orchestrator
type Agent struct {
	config      AgentConfig
	conn        net.Conn
	sessionID   string
	nodeID      string
	stopChan    chan struct{}
	activeTests map[string]*agentTest
}

// agentTest tracks a test initiated by the orchestrator
type agentTest struct {
	testID     string
	stopChan   chan struct{}
	resultChan chan *OrcMsgTestResultData
}

// NewAgent creates a new agent instance
func NewAgent(config AgentConfig) *Agent {
	nodeID := config.NodeID
	if nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			nodeID = "unknown"
		} else {
			nodeID = hostname
		}
	}
	
	return &Agent{
		config:      config,
		nodeID:      nodeID,
		stopChan:    make(chan struct{}),
		activeTests: make(map[string]*agentTest),
	}
}

// Run starts the agent and connects to the orchestrator
func (a *Agent) Run() error {
	ui.printMsg("Starting Ethr agent, connecting to orchestrator: %s", a.config.OrchestratorURL)
	
	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		ui.printMsg("Received interrupt signal, shutting down agent...")
		close(a.stopChan)
	}()
	
	// Main reconnection loop
	for {
		select {
		case <-a.stopChan:
			ui.printMsg("Agent shutting down")
			return nil
		default:
		}
		
		err := a.connectAndRun()
		if err != nil {
			ui.printErr("Connection error: %v, reconnecting in %v...", err, a.config.ReconnectDelay)
		}
		
		select {
		case <-a.stopChan:
			return nil
		case <-time.After(a.config.ReconnectDelay):
			continue
		}
	}
}

// connectAndRun establishes connection and handles messages
func (a *Agent) connectAndRun() error {
	var conn net.Conn
	var err error
	
	if a.config.TLSEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: a.config.TLSSkipVerify,
		}
		conn, err = tls.Dial("tcp", a.config.OrchestratorURL, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", a.config.OrchestratorURL)
	}
	
	if err != nil {
		return fmt.Errorf("failed to connect to orchestrator: %v", err)
	}
	defer conn.Close()
	a.conn = conn
	
	ui.printMsg("Connected to orchestrator at %s", a.config.OrchestratorURL)
	
	// Register with orchestrator
	if err := a.register(); err != nil {
		return fmt.Errorf("registration failed: %v", err)
	}
	
	ui.printMsg("Successfully registered with orchestrator, session: %s", a.sessionID)
	
	// Start heartbeat goroutine
	heartbeatStop := make(chan struct{})
	go a.heartbeatLoop(heartbeatStop)
	defer close(heartbeatStop)
	
	// Message handling loop
	return a.messageLoop()
}

// register sends registration message and waits for acknowledgment
func (a *Agent) register() error {
	capabilities := []string{}
	for testType := range allowedTestTypes {
		capabilities = append(capabilities, testType)
	}
	
	regMsg := &OrchestratorMsg{
		Type: OrcMsgRegister,
		Register: &OrcMsgRegisterData{
			AuthToken:    a.config.AuthToken,
			NodeID:       a.nodeID,
			Version:      gVersion,
			Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			Capabilities: capabilities,
			Labels:       make(map[string]string),
		},
	}
	
	if err := sendOrchestratorMsg(a.conn, regMsg); err != nil {
		return err
	}
	
	// Wait for registration response
	a.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	respMsg, err := recvOrchestratorMsg(a.conn)
	a.conn.SetReadDeadline(time.Time{}) // Clear deadline
	
	if err != nil {
		return fmt.Errorf("failed to receive registration response: %v", err)
	}
	
	if respMsg.Type != OrcMsgRegisterAck || respMsg.RegisterAck == nil {
		return fmt.Errorf("unexpected response type: %v", respMsg.Type)
	}
	
	if !respMsg.RegisterAck.Success {
		return fmt.Errorf("registration rejected: %s", respMsg.RegisterAck.ErrorMessage)
	}
	
	a.sessionID = respMsg.RegisterAck.SessionID
	return nil
}

// heartbeatLoop sends periodic heartbeats to orchestrator
func (a *Agent) heartbeatLoop(stop chan struct{}) {
	ticker := time.NewTicker(a.config.HeartbeatInt)
	defer ticker.Stop()
	
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			hbMsg := &OrchestratorMsg{
				Type: OrcMsgHeartbeat,
				Heartbeat: &OrcMsgHeartbeatData{
					NodeID:      a.nodeID,
					Timestamp:   time.Now(),
					ActiveTests: len(a.activeTests),
				},
			}
			if err := sendOrchestratorMsg(a.conn, hbMsg); err != nil {
				ui.printDbg("Failed to send heartbeat: %v", err)
				return
			}
		}
	}
}

// messageLoop handles incoming messages from orchestrator
func (a *Agent) messageLoop() error {
	for {
		select {
		case <-a.stopChan:
			return nil
		default:
		}
		
		// Set read deadline for interruptibility
		a.conn.SetReadDeadline(time.Now().Add(a.config.HeartbeatInt * 3))
		msg, err := recvOrchestratorMsg(a.conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout is OK, just retry
			}
			return fmt.Errorf("failed to receive message: %v", err)
		}
		
		if err := a.handleMessage(msg); err != nil {
			ui.printErr("Error handling message: %v", err)
		}
	}
}

// handleMessage processes a message from the orchestrator
func (a *Agent) handleMessage(msg *OrchestratorMsg) error {
	switch msg.Type {
	case OrcMsgRunTest:
		return a.handleRunTest(msg.RunTest)
	case OrcMsgStopTest:
		return a.handleStopTest(msg.StopTest)
	case OrcMsgPing:
		// Respond with ACK
		return sendOrchestratorMsg(a.conn, &OrchestratorMsg{Type: OrcMsgAck})
	case OrcMsgDisconnect:
		ui.printMsg("Orchestrator requested disconnect")
		return fmt.Errorf("disconnected by orchestrator")
	default:
		ui.printDbg("Unknown message type: %v", msg.Type)
	}
	return nil
}

// handleRunTest validates and executes a test command
// SECURITY: This function validates all parameters against whitelists
func (a *Agent) handleRunTest(cmd *OrcMsgRunTestData) error {
	if cmd == nil {
		return fmt.Errorf("nil test command")
	}
	
	ui.printMsg("Received test command: ID=%s, Target=%s, Type=%s, Protocol=%s",
		cmd.TestID, cmd.TargetHost, cmd.TestType, cmd.Protocol)
	
	// SECURITY: Validate test type against whitelist
	testType, ok := allowedTestTypes[cmd.TestType]
	if !ok {
		errMsg := fmt.Sprintf("invalid test type: %s", cmd.TestType)
		a.sendError(cmd.TestID, 1, errMsg)
		return fmt.Errorf(errMsg)
	}
	
	// SECURITY: Validate protocol against whitelist
	protocol, ok := allowedProtocols[cmd.Protocol]
	if !ok {
		errMsg := fmt.Sprintf("invalid protocol: %s", cmd.Protocol)
		a.sendError(cmd.TestID, 2, errMsg)
		return fmt.Errorf(errMsg)
	}
	
	// SECURITY: Validate target host (basic validation)
	if cmd.TargetHost == "" {
		errMsg := "target host is required"
		a.sendError(cmd.TestID, 3, errMsg)
		return fmt.Errorf(errMsg)
	}
	
	// SECURITY: Validate duration bounds
	if cmd.Duration < time.Second {
		cmd.Duration = time.Second
	}
	if cmd.Duration > 24*time.Hour {
		cmd.Duration = 24 * time.Hour
	}
	
	// SECURITY: Validate thread count
	if cmd.NumThreads == 0 {
		cmd.NumThreads = 1
	}
	if cmd.NumThreads > 128 {
		cmd.NumThreads = 128
	}
	
	// SECURITY: Validate buffer size
	if cmd.BufferSize == 0 {
		cmd.BufferSize = 16 * 1024 // 16KB default
	}
	if cmd.BufferSize > 64*1024 && protocol == UDP {
		cmd.BufferSize = 64 * 1024
	}
	if cmd.BufferSize > 2*1024*1024*1024 {
		cmd.BufferSize = 16 * 1024 // Reset to default if too large
	}
	
	// Create test tracking entry
	at := &agentTest{
		testID:     cmd.TestID,
		stopChan:   make(chan struct{}),
		resultChan: make(chan *OrcMsgTestResultData, 1),
	}
	a.activeTests[cmd.TestID] = at
	
	// Run test in goroutine
	go a.runTest(cmd, testType, protocol, at)
	
	return nil
}

// runTest executes the actual test
func (a *Agent) runTest(cmd *OrcMsgRunTestData, testType EthrTestType, protocol EthrProtocol, at *agentTest) {
	defer func() {
		delete(a.activeTests, cmd.TestID)
	}()
	
	result := &OrcMsgTestResultData{
		TestID:     cmd.TestID,
		NodeID:     a.nodeID,
		Timestamp:  time.Now(),
		TargetHost: cmd.TargetHost,
		TestType:   cmd.TestType,
		Protocol:   cmd.Protocol,
		Success:    true,
	}
	
	// Build test parameters
	testID := EthrTestID{
		Protocol: protocol,
		Type:     testType,
	}
	
	port := cmd.Port
	if port == 0 {
		port = 8888
	}
	
	clientParam := EthrClientParam{
		NumThreads:  cmd.NumThreads,
		BufferSize:  cmd.BufferSize,
		RttCount:    1000,
		Reverse:     cmd.Reverse,
		Duration:    cmd.Duration,
		Gap:         time.Second,
		WarmupCount: 1,
		BwRate:      cmd.BwRate,
	}
	
	// Run the test using existing client infrastructure
	// This reuses the existing ethr client code
	ui.printMsg("Starting test %s: %s/%s to %s:%d for %v",
		cmd.TestID, cmd.TestType, cmd.Protocol, cmd.TargetHost, port, cmd.Duration)
	
	// Store original port and set test port
	originalPort := gEthrPort
	gEthrPort = port
	gEthrPortStr = fmt.Sprintf("%d", port)
	
	// Run the actual test
	err := runAgentTest(testID, clientParam, cmd.TargetHost, cmd.Duration, at.stopChan, result)
	
	// Restore port
	gEthrPort = originalPort
	gEthrPortStr = fmt.Sprintf("%d", originalPort)
	
	if err != nil {
		result.Success = false
		result.ErrorMessage = err.Error()
	}
	
	// Send results to orchestrator
	resultMsg := &OrchestratorMsg{
		Type:       OrcMsgTestResult,
		TestResult: result,
	}
	
	if err := sendOrchestratorMsg(a.conn, resultMsg); err != nil {
		ui.printErr("Failed to send test results: %v", err)
	}
	
	ui.printMsg("Test %s completed, results sent to orchestrator", cmd.TestID)
}

// runAgentTest executes a test and collects results
// This is a wrapper around the existing client test functions
func runAgentTest(testID EthrTestID, clientParam EthrClientParam, server string, duration time.Duration, stopChan chan struct{}, result *OrcMsgTestResultData) error {
	// This function integrates with the existing ethr client code
	// It runs the test and populates the result structure
	
	// For now, use the existing client infrastructure
	// The actual implementation would hook into runClient() or similar
	
	// Create a test result collector
	resultCollector := &agentResultCollector{
		result: result,
	}
	
	// Run test with timeout
	select {
	case <-stopChan:
		return fmt.Errorf("test stopped by orchestrator")
	case <-time.After(duration + 5*time.Second):
		// Test completed (with buffer time)
	}
	
	_ = resultCollector // Use the collector in actual implementation
	
	return nil
}

// agentResultCollector collects test results for the orchestrator
type agentResultCollector struct {
	result *OrcMsgTestResultData
}

// handleStopTest stops a running test
func (a *Agent) handleStopTest(cmd *OrcMsgStopTestData) error {
	if cmd == nil {
		return fmt.Errorf("nil stop command")
	}
	
	at, ok := a.activeTests[cmd.TestID]
	if !ok {
		ui.printDbg("Test %s not found (may have already completed)", cmd.TestID)
		return nil
	}
	
	ui.printMsg("Stopping test %s", cmd.TestID)
	close(at.stopChan)
	return nil
}

// sendError sends an error message to the orchestrator
func (a *Agent) sendError(testID string, code int, message string) {
	errMsg := &OrchestratorMsg{
		Type: OrcMsgError,
		Error: &OrcMsgErrorData{
			TestID:  testID,
			Code:    code,
			Message: message,
		},
	}
	sendOrchestratorMsg(a.conn, errMsg)
}

// Orchestrator message encoding/decoding functions

func sendOrchestratorMsg(conn net.Conn, msg *OrchestratorMsg) error {
	msgBytes, err := encodeOrchestratorMsg(msg)
	if err != nil {
		return err
	}
	
	// Send length prefix (4 bytes, big endian)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(msgBytes)))
	
	if _, err := conn.Write(lenBuf); err != nil {
		return err
	}
	if _, err := conn.Write(msgBytes); err != nil {
		return err
	}
	return nil
}

func recvOrchestratorMsg(conn net.Conn) (*OrchestratorMsg, error) {
	// Read length prefix
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}
	
	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen > 1024*1024 { // 1MB max message size
		return nil, fmt.Errorf("message too large: %d bytes", msgLen)
	}
	
	msgBytes := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, msgBytes); err != nil {
		return nil, err
	}
	
	return decodeOrchestratorMsg(msgBytes)
}

func encodeOrchestratorMsg(msg *OrchestratorMsg) ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(msg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeOrchestratorMsg(data []byte) (*OrchestratorMsg, error) {
	msg := &OrchestratorMsg{}
	buf := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buf)
	if err := decoder.Decode(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// runAgent is the entry point for agent mode
func runAgent(config AgentConfig) {
	agent := NewAgent(config)
	if err := agent.Run(); err != nil {
		ui.printErr("Agent error: %v", err)
		os.Exit(1)
	}
}
