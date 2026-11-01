package config

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestStreamAMIEventsReceivesVoiceAndCancels(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	login := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "Asterisk Call Manager\r\n")
		reader := bufio.NewReader(conn)
		message, _ := readAMIMessage(reader)
		login <- message
		fmt.Fprint(conn, "Response: Success\r\n\r\nEvent: ChannelTalkingStart\r\nChannel: PJSIP/1001-00000001\r\nUniqueid: call1\r\nExten: 616262\r\n\r\nEvent: ChannelTalkingStop\r\nChannel: PJSIP/1001-00000001\r\nUniqueid: call1\r\nExten: 616262\r\n\r\n")
		readAMIMessage(reader)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan string, 2)
	done := make(chan error, 1)
	go func() {
		done <- StreamAMIEvents(ctx, host, port, "test", "test", func(fields map[string]string) {
			events <- fields["Event"]
		})
	}()
	for _, want := range []string{"ChannelTalkingStart", "ChannelTalkingStop"} {
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("event=%s want=%s", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("AMI event not received")
		}
	}
	if !strings.Contains(<-login, "Events: call,dialplan") {
		t.Fatal("voice event classes were not requested")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("AMI stream did not cancel")
	}
}
