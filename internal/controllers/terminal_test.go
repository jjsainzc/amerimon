package controllers

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestValidateTerminalConnectionRequest(t *testing.T) {
	valid := terminalConnectionRequest{
		Type:       "connect",
		Host:       "192.0.2.10",
		Port:       22,
		Username:   "amerimon",
		AuthMethod: "password",
		Password:   "secret",
	}
	if err := validateTerminalConnectionRequest(valid); err != nil {
		t.Fatalf("valid password request rejected: %v", err)
	}

	keyRequest := valid
	keyRequest.AuthMethod = "publickey"
	keyRequest.Password = ""
	keyRequest.PrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nkey\n"
	if err := validateTerminalConnectionRequest(keyRequest); err != nil {
		t.Fatalf("valid public-key request rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*terminalConnectionRequest)
	}{
		{"missing host", func(r *terminalConnectionRequest) { r.Host = "" }},
		{"host with path", func(r *terminalConnectionRequest) { r.Host = "host.example/path" }},
		{"invalid port", func(r *terminalConnectionRequest) { r.Port = 65536 }},
		{"missing username", func(r *terminalConnectionRequest) { r.Username = "" }},
		{"missing password", func(r *terminalConnectionRequest) { r.Password = "" }},
		{"missing key", func(r *terminalConnectionRequest) { r.AuthMethod = "publickey"; r.PrivateKey = "" }},
		{"unsupported auth", func(r *terminalConnectionRequest) { r.AuthMethod = "agent" }},
		{"invalid message type", func(r *terminalConnectionRequest) { r.Type = "input" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.edit(&request)
			if err := validateTerminalConnectionRequest(request); err == nil {
				t.Fatal("invalid request was accepted")
			}
		})
	}
}

func TestValidTerminalSize(t *testing.T) {
	cols, rows := validTerminalSize(100, 30)
	if cols != 100 || rows != 30 {
		t.Fatalf("valid terminal size changed to %dx%d", cols, rows)
	}
	cols, rows = validTerminalSize(1, 900)
	if cols != 80 || rows != 24 {
		t.Fatalf("invalid terminal size was not reset to 80x24: %dx%d", cols, rows)
	}
}

func TestSaveTerminalHostKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	callback, path, err := terminalHostKeyCallback()
	if err != nil {
		t.Fatalf("prepare known_hosts: %v", err)
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatalf("encode host key: %v", err)
	}

	host := "127.0.0.1:2222"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	err = callback(host, remote, publicKey)
	var keyErr *knownhosts.KeyError
	var untrusted *unknownTerminalHostKey
	if !errors.As(err, &untrusted) {
		t.Fatalf("expected unknown host key, got %v", err)
	}
	if err := saveTerminalHostKey(path, host, publicKey); err != nil {
		t.Fatalf("save host key: %v", err)
	}
	callback, _, err = terminalHostKeyCallback()
	if err != nil {
		t.Fatalf("reload known_hosts: %v", err)
	}
	if err := callback(host, remote, publicKey); err != nil {
		t.Fatalf("saved host key was not accepted: %v", err)
	}
	otherPublicKey, err := ssh.NewPublicKey(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)))
	if err != nil {
		t.Fatalf("encode different host key: %v", err)
	}
	err = callback(host, remote, otherPublicKey)
	if !errors.As(err, &keyErr) || len(keyErr.Want) == 0 {
		t.Fatalf("changed host key was not rejected: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat known_hosts: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("known_hosts permissions = %o, want 600", info.Mode().Perm())
	}
	sshDirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat .ssh: %v", err)
	}
	if sshDirInfo.Mode().Perm() != 0700 {
		t.Fatalf(".ssh permissions = %o, want 700", sshDirInfo.Mode().Perm())
	}
}

func TestTerminalHostKeyCallbackDistinguishesUnknownAndChangedKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	callback, _, err := terminalHostKeyCallback()
	if err != nil {
		t.Fatalf("prepare known_hosts: %v", err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatalf("encode host key: %v", err)
	}

	host := "127.0.0.1:22"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	err = callback(host, remote, publicKey)
	var untrusted *unknownTerminalHostKey
	if !errors.As(err, &untrusted) {
		t.Fatalf("unknown host should request explicit trust, got %v", err)
	}
	if untrusted.host != host || !bytes.Equal(untrusted.key.Marshal(), publicKey.Marshal()) {
		t.Fatalf("unknown host prompt has incorrect key details: %#v", untrusted)
	}

	if err := saveTerminalHostKey(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"), host, publicKey); err != nil {
		t.Fatalf("save known host key: %v", err)
	}
	callback, _, err = terminalHostKeyCallback()
	if err != nil {
		t.Fatalf("reload known_hosts: %v", err)
	}
	_, otherPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate replacement host key: %v", err)
	}
	otherKey, err := ssh.NewPublicKey(otherPrivateKey.Public())
	if err != nil {
		t.Fatalf("encode replacement host key: %v", err)
	}
	err = callback(host, remote, otherKey)
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) || len(keyErr.Want) == 0 {
		t.Fatalf("changed host key should be rejected, got %v", err)
	}
}

func TestTerminalUserHomeDirFallsBackWhenHomeIsUnset(t *testing.T) {
	t.Setenv("HOME", "")
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("get current user: %v", err)
	}

	home, err := terminalUserHomeDir()
	if err != nil {
		t.Fatalf("resolve service user's home: %v", err)
	}
	if home != currentUser.HomeDir {
		t.Fatalf("home = %q, want current user's home %q", home, currentUser.HomeDir)
	}
}

func TestTerminalConnectionErrorExplainsAuthenticationMethod(t *testing.T) {
	request := terminalConnectionRequest{
		Host:       "10.0.0.8",
		Username:   "operator",
		AuthMethod: "publickey",
	}
	message := terminalConnectionError(errors.New(
		"ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain",
	), request)
	if !strings.Contains(message, "10.0.0.8") ||
		!strings.Contains(message, "clave pública") ||
		!strings.Contains(message, "clave pública autorizada") {
		t.Fatalf("public-key authentication error is not actionable: %q", message)
	}

	request.AuthMethod = "password"
	message = terminalConnectionError(errors.New("ssh: unable to authenticate"), request)
	if !strings.Contains(message, "rechazó la contraseña") {
		t.Fatalf("password authentication error is not specific: %q", message)
	}
}

func TestTerminalAuthPreparationExplainsMissingPassphrase(t *testing.T) {
	message := terminalAuthPreparationError(&ssh.PassphraseMissingError{}, "publickey")
	if !strings.Contains(message, "frase de contraseña") {
		t.Fatalf("missing key passphrase error is not actionable: %q", message)
	}
}
