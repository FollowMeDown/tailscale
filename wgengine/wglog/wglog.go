// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package wglog contains logging helpers for wireguard-go.
package wglog

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/tailscale/wireguard-go/device"
	"tailscale.com/types/logger"
	"tailscale.com/wgengine/wgcfg"
)

// A Logger is a wireguard-go log wrapper that cleans up and rewrites log lines.
// It can be modified at run time to adjust to new wireguard-go configurations.
type Logger struct {
	DeviceLogger *device.Logger
	replacer     atomic.Value // of *strings.Replacer
}

// NewLogger creates a new logger for use with wireguard-go.
// This logger silences repetitive/unhelpful noisy log lines
// and rewrites peer keys from wireguard-go into Tailscale format.
func NewLogger(logf logger.Logf) *Logger {
	ret := new(Logger)

	wrapper := func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		if strings.Contains(msg, "Routine:") {
			// wireguard-go logs as it starts and stops routines.
			// Drop those; there are a lot of them, and they're just noise.
			return
		}
		if strings.Contains(msg, "Failed to send data packet") {
			// Drop. See https://github.com/tailscale/tailscale/issues/1239.
			return
		}
		r := ret.replacer.Load()
		if r == nil {
			// No replacements specified; log as originally planned.
			logf(format, args...)
			return
		}
		// Do the replacements.
		new := r.(*strings.Replacer).Replace(msg)
		if new == msg {
			// No replacements. Log as originally planned.
			logf(format, args...)
			return
		}
		// We made some replacements. Log the new version.
		// This changes the format string,
		// which is somewhat unfortunate as it impacts rate limiting,
		// but there's not much we can do about that.
		logf("%s", new)
	}
	std := logger.StdLogger(wrapper)
	ret.DeviceLogger = &device.Logger{
		Debug: std,
		Info:  std,
		Error: std,
	}
	return ret
}

// SetPeers adjusts x to rewrite the peer public keys found in peers.
// SetPeers is safe for concurrent use.
func (x *Logger) SetPeers(peers []wgcfg.Peer) {
	// Construct a new peer public key log rewriter.
	var replace []string
	for _, peer := range peers {
		old := "peer(" + wireguardGoString(peer.PublicKey) + ")"
		new := peer.PublicKey.ShortString()
		replace = append(replace, old, new)
	}
	r := strings.NewReplacer(replace...)
	x.replacer.Store(r)
}

// wireguardGoString prints p in the same format used by wireguard-go.
func wireguardGoString(k wgcfg.Key) string {
	base64Key := base64.StdEncoding.EncodeToString(k[:])
	abbreviatedKey := "invalid"
	if len(base64Key) == 44 {
		abbreviatedKey = base64Key[0:4] + "…" + base64Key[39:43]
	}
	return abbreviatedKey
}
