// Copyright 2026 The HuaTuo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transport

import (
	"errors"
	"net"
	"testing"
	"time"
)

// handshakeFailConn fails every write and records whether Close was called, so the
// handshake-failure cleanup path can be observed deterministically.
type handshakeFailConn struct {
	net.Conn
	writeErr error
	closed   bool
}

func (c *handshakeFailConn) Write([]byte) (int, error) { return 0, c.writeErr }

func (c *handshakeFailConn) Close() error {
	c.closed = true
	return nil
}

func (c *handshakeFailConn) SetDeadline(time.Time) error      { return nil }
func (c *handshakeFailConn) SetReadDeadline(time.Time) error  { return nil }
func (c *handshakeFailConn) SetWriteDeadline(time.Time) error { return nil }

// A handshake failure must close the dialed connection: the caller never
// receives the Client, so without an explicit close the fd lives until the
// finalizer runs.
func TestNewClientClosesConnOnHandshakeFailure(t *testing.T) {
	var captured *handshakeFailConn
	orig := dialUDS
	dialUDS = func(string) (net.Conn, error) {
		captured = &handshakeFailConn{writeErr: errors.New("server went away")}
		return captured, nil
	}
	t.Cleanup(func() { dialUDS = orig })

	client, err := NewClient("/unused.sock", "tool", "1", "task")
	if err == nil {
		if client != nil {
			_ = client.Close()
		}
		t.Fatal("NewClient() error=nil, want handshake failure")
	}
	if client != nil {
		t.Error("NewClient() returned a client alongside an error")
	}
	if captured == nil {
		t.Fatal("dial seam was not invoked")
	}
	if !captured.closed {
		t.Error("connection was not closed on handshake failure; fd leaks until the finalizer runs")
	}
}
