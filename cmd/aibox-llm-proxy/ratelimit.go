package main

import (
	"sync"
	"time"
)

const slidingWindow = 60 * time.Second

type tokenEntry struct {
	time   time.Time
	tokens int
}

// RateLimiter enforces sliding-window rate limits for both requests-per-minute
// (RPM) and tokens-per-minute (TPM).
type RateLimiter struct {
	mu           sync.Mutex
	requestTimes []time.Time
	tokenCounts  []tokenEntry
	maxRPM       int
	maxTPM       int
}

// NewRateLimiter creates a rate limiter with the given RPM and TPM limits.
func NewRateLimiter(maxRPM, maxTPM int) *RateLimiter {
	return &RateLimiter{
		maxRPM: maxRPM,
		maxTPM: maxTPM,
	}
}

// AllowRequest checks whether a new request is allowed under the RPM limit.
// If allowed, it records the request timestamp.
func (rl *RateLimiter) AllowRequest() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-slidingWindow)

	// Prune old entries
	rl.requestTimes = pruneTimestamps(rl.requestTimes, cutoff)

	if len(rl.requestTimes) >= rl.maxRPM {
		return false
	}
	rl.requestTimes = append(rl.requestTimes, now)
	return true
}

// RecordTokens records estimated token usage for TPM tracking.
func (rl *RateLimiter) RecordTokens(tokens int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.tokenCounts = append(rl.tokenCounts, tokenEntry{
		time:   time.Now(),
		tokens: tokens,
	})
}

// AllowTokens checks whether the given token count would exceed the TPM limit.
func (rl *RateLimiter) AllowTokens(tokens int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-slidingWindow)
	rl.tokenCounts = pruneTokenEntries(rl.tokenCounts, cutoff)

	total := 0
	for _, e := range rl.tokenCounts {
		total += e.tokens
	}
	return total+tokens <= rl.maxTPM
}

func pruneTimestamps(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	if i == 0 {
		return ts
	}
	n := copy(ts, ts[i:])
	return ts[:n]
}

func pruneTokenEntries(entries []tokenEntry, cutoff time.Time) []tokenEntry {
	i := 0
	for i < len(entries) && entries[i].time.Before(cutoff) {
		i++
	}
	if i == 0 {
		return entries
	}
	n := copy(entries, entries[i:])
	return entries[:n]
}
