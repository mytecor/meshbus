//go:build linux

package rns

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mytecor/meshbus/core"
)

const defaultSharedInstanceSocket = "@rns/default"

// TestReticulumGoDaemonSharedInstance is deliberately black-box: the pinned
// Reticulum-Go daemon runs in another process and meshbus only uses its public,
// production shared-instance endpoint.
func TestReticulumGoDaemonSharedInstance(t *testing.T) {
	if sharedInstanceListening() {
		t.Skipf("%s is already in use", defaultSharedInstanceSocket)
	}
	assertProductionSharedInstanceFailsClosed(t)

	daemon := startReticulumGoDaemon(t)
	t.Cleanup(daemon.stop)

	received := make(chan core.ReceivedMessage, 64)
	observer := newDaemonEndpoint(t, true, nil)
	receiver := newDaemonEndpoint(t, false, func(_ context.Context, message core.ReceivedMessage) error {
		received <- message
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	startDaemonEndpoint(t, ctx, observer)
	startDaemonEndpoint(t, ctx, receiver)
	waitForPeer(t, observer, receiver.Name())

	// This client missed the receiver's announce. Supplying the known RNS
	// destination must trigger PathRequest; meshbus does not discover services
	// by aspect or invent a peer before the caller supplies a destination.
	late := newDaemonEndpoint(t, true, nil)
	startDaemonEndpoint(t, ctx, late)
	if peers := late.DiscoveredPeers(); len(peers) != 0 {
		t.Fatalf("late client unexpectedly discovered peers: %+v", peers)
	}

	sendCtx, stopSending := context.WithTimeout(ctx, 30*time.Second)
	defer stopSending()
	target := receiver.Destination()
	assertDaemonDelivery(t, sendCtx, late, target, []byte("late-channel"), received)

	resourcePayload := daemonTestPayload(32 * 1024)
	assertDaemonDelivery(t, sendCtx, late, target, resourcePayload, received)

	// Tear down the authenticated link. The next send must establish and
	// authenticate a fresh Link/Channel session over the cached route.
	destinationHash, key, err := parseDestination(target)
	if err != nil {
		t.Fatal(err)
	}
	_, _, active := late.connections.resolve(destinationHash, key)
	if active == nil {
		t.Fatal("outbound authenticated session was not cached")
	}
	active.link.Teardown()
	assertDaemonDelivery(t, sendCtx, late, target, []byte("after-reconnect"), received)

	const concurrentSends = 8
	errorsSeen := make(chan error, concurrentSends)
	want := make(map[string][]byte, concurrentSends)
	var sends sync.WaitGroup
	for index := range concurrentSends {
		payload := []byte(fmt.Sprintf("concurrent-channel-%d", index))
		if index%2 != 0 {
			payload = daemonTestPayload(32*1024 + index)
		}
		want[string(payload)] = payload
		sends.Add(1)
		go func(payload []byte) {
			defer sends.Done()
			if err := late.SendToDestination(sendCtx, target, payload); err != nil {
				errorsSeen <- err
			}
		}(payload)
	}
	sends.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Errorf("concurrent daemon send: %v", err)
	}
	for range concurrentSends {
		select {
		case message := <-received:
			key := string(message.Payload())
			if _, ok := want[key]; !ok {
				t.Fatalf("unexpected concurrent payload: %x", message.Payload())
			}
			delete(want, key)
		case <-sendCtx.Done():
			t.Fatalf("waiting for concurrent daemon delivery: %v", sendCtx.Err())
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing %d concurrent daemon deliveries", len(want))
	}
}

func assertProductionSharedInstanceFailsClosed(t *testing.T) {
	t.Helper()
	endpoint := newDaemonEndpoint(t, true, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := endpoint.Start(ctx); !errors.Is(err, ErrSharedInstanceUnavailable) {
		t.Fatalf("Start() error = %v, want %v", err, ErrSharedInstanceUnavailable)
	}
	_ = endpoint.Close()

	listener, err := net.Listen("unix", defaultSharedInstanceSocket)
	if err != nil {
		t.Fatalf("failed shared-client attach claimed %s: %v", defaultSharedInstanceSocket, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

type daemonProcess struct {
	cmd     *exec.Cmd
	wait    chan error
	logPath string
}

func startReticulumGoDaemon(t *testing.T) *daemonProcess {
	t.Helper()
	root := t.TempDir()
	binary := filepath.Join(root, "reticulum-go")
	module := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/Quad4-Software/Reticulum-Go")
	moduleOutput, err := module.CombinedOutput()
	if err != nil {
		t.Fatalf("locate Reticulum-Go module: %v\n%s", err, moduleOutput)
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/reticulum-go")
	build.Dir = strings.TrimSpace(string(moduleOutput))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Reticulum-Go daemon: %v\n%s", err, output)
	}

	configPath := filepath.Join(root, "config")
	config := []byte("[reticulum]\n  enable_transport = yes\n  share_instance = yes\n  shared_instance_type = unix\n  instance_name = default\n  enable_sandbox = no\n  enable_seccomp = no\n  in_memory_storage = yes\n\n[logging]\n  loglevel = 2\n")
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "daemon.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binary, "--config", configPath)
	cmd.Env = append(os.Environ(), "HOME="+root, "RETICULUM_IN_MEMORY_STORAGE=1")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatal(err)
	}
	process := &daemonProcess{cmd: cmd, wait: make(chan error, 1), logPath: logPath}
	go func() {
		process.wait <- cmd.Wait()
		_ = logFile.Close()
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if sharedInstanceListening() {
			return process
		}
		select {
		case err := <-process.wait:
			t.Fatalf("Reticulum-Go daemon exited before readiness: %v\n%s", err, readDaemonLog(logPath))
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	process.stop()
	t.Fatalf("Reticulum-Go daemon did not listen on %s\n%s", defaultSharedInstanceSocket, readDaemonLog(logPath))
	return nil
}

func (p *daemonProcess) stop() {
	if p == nil || p.cmd == nil || p.cmd.Process == nil || p.cmd.ProcessState != nil {
		return
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-p.wait:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.wait
	}
}

func sharedInstanceListening() bool {
	connection, err := net.DialTimeout("unix", defaultSharedInstanceSocket, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func readDaemonLog(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func newDaemonEndpoint(t *testing.T, passive bool, handler core.Handler) *Endpoint {
	t.Helper()
	if handler == nil {
		handler = func(context.Context, core.ReceivedMessage) error { return nil }
	}
	endpoint, err := New(Config{
		StackMode:         StackSharedClient,
		EphemeralIdentity: true,
		RealmKey:          testRealmKey(),
		Passive:           passive,
		NetworkWait:       15 * time.Second,
		Handler:           handler,
	})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func startDaemonEndpoint(t *testing.T, ctx context.Context, endpoint *Endpoint) {
	t.Helper()
	if err := endpoint.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = endpoint.Close() })
	if endpoint.stack.shared == nil || endpoint.stack.shared.Client == nil {
		t.Fatal("endpoint did not attach as a shared-instance client")
	}
}

func assertDaemonDelivery(t *testing.T, ctx context.Context, sender *Endpoint, target string, payload []byte, received <-chan core.ReceivedMessage) {
	t.Helper()
	if err := sender.SendToDestination(ctx, target, payload); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-received:
		if message.Sender().String() != sender.Name() || !bytes.Equal(message.Payload(), payload) {
			t.Fatalf("daemon delivery sender=%s payload-bytes=%d", message.Sender(), len(message.Payload()))
		}
	case <-ctx.Done():
		t.Fatalf("waiting for daemon delivery: %v", ctx.Err())
	}
}

func daemonTestPayload(size int) []byte {
	payload := make([]byte, size)
	state := uint32(0x726e7364)
	for index := range payload {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		payload[index] = byte(state)
	}
	return payload
}
