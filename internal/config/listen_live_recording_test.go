package config

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type testAudioConsumer struct {
	mu     sync.Mutex
	data   bytes.Buffer
	closed chan struct{}
	once   sync.Once
	fail   bool
}

func (s *testAudioConsumer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return 0, errors.New("recording disk error")
	}
	return s.data.Write(p)
}

func (s *testAudioConsumer) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func TestEAGIServerConsumerReceivesPCMWithoutBrowserAndFinalizesOnDisconnect(t *testing.T) {
	manager := NewAudioStreamManager()
	uuid := "a1234567-1234-1234-1234-123456789abc"
	consumer := &testAudioConsumer{closed: make(chan struct{})}
	manager.AddConsumer(uuid, consumer)
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		HandleEAGIConnection(server, manager)
		close(done)
	}()
	payload := bytes.Repeat([]byte{1, 2}, 160)
	if _, err := client.Write([]byte(uuid)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(payload); err != nil {
		t.Fatal(err)
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("EAGI stream did not finalize")
	}
	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	if !bytes.Equal(consumer.data.Bytes(), payload) {
		t.Fatal("server recording did not receive exact PCM")
	}
	select {
	case <-consumer.closed:
	default:
		t.Fatal("recording not finalized on stream disconnect")
	}
}

func TestAudioServerConsumerCountsAsListenerAndFailureRemovesIt(t *testing.T) {
	manager := NewAudioStreamManager()
	consumer := &testAudioConsumer{closed: make(chan struct{})}
	manager.AddConsumer("recording", consumer)
	if count := manager.Broadcast("recording", []byte{1, 2}); count != 1 {
		t.Fatalf("server capture was not counted as listener: %d", count)
	}
	consumer.fail = true
	if count := manager.Broadcast("recording", []byte{1, 2}); count != 0 {
		t.Fatalf("failed capture still counted as listener: %d", count)
	}
	select {
	case <-consumer.closed:
	default:
		t.Fatal("failed recording not closed")
	}
	manager.Broadcast("recording", []byte{1, 2})
	manager.CloseClients("recording")
}
